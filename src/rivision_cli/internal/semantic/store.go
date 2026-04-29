package semantic

import (
	"database/sql"
	"log"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// HistoryItem 对齐 gateway /api/v1/inference/history 的字段
// timestamp 支持 RFC3339 或 Unix 秒（由 parseTimestamp 兼容处理）
type HistoryItem struct {
	TaskID         string      `json:"task_id"`
	Timestamp      interface{} `json:"timestamp"`
	CameraID       string      `json:"camera_id"`
	StreamName     string      `json:"stream_name"`
	Thumbnail      string      `json:"thumbnail"`
	Result         string      `json:"result"`
	NodeID         string      `json:"node_id"`
	ProcessingTime int64       `json:"processing_time"`
	Resolution     string      `json:"resolution"`
}

type SearchResult struct {
	TaskID         string    `json:"task_id"`
	Timestamp      time.Time `json:"timestamp"`
	CameraID       string    `json:"camera_id"`
	StreamName     string    `json:"stream_name,omitempty"`
	Thumbnail      string    `json:"thumbnail,omitempty"`
	Result         string    `json:"result"`
	NodeID         string    `json:"node_id,omitempty"`
	ProcessingTime int64     `json:"processing_time,omitempty"`
	Resolution     string    `json:"resolution,omitempty"`
	Score          float64   `json:"score"`
	TextScore      float64   `json:"text_score,omitempty"`
	ImageScore     float64   `json:"image_score,omitempty"`
}

type Store struct {
	mu        sync.Mutex
	db        *sql.DB
	vectorDim int
}

func NewStore(baseDir string) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(baseDir, "semantic_search.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	
	// 启用 WAL 模式，提高并发写入性能，避免 SQLITE_BUSY
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		log.Printf("[Store] WAL mode failed: %v", err)
	}
	// 设置 busy_timeout，等待锁释放而非立即失败
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		log.Printf("[Store] busy_timeout failed: %v", err)
	}
	// 同步模式设为 NORMAL，平衡性能和安全
	if _, err := db.Exec(`PRAGMA synchronous=NORMAL`); err != nil {
		log.Printf("[Store] synchronous failed: %v", err)
	}
	
	s := &Store{db: db, vectorDim: 512}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	log.Printf("[Store] SQLite opened with WAL mode: %s", dbPath)
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) initSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS vlm_semantic_cache (
  task_id TEXT PRIMARY KEY,
  timestamp_unix INTEGER NOT NULL,
  camera_id TEXT NOT NULL,
  stream_name TEXT,
  thumbnail TEXT,
  result_text TEXT NOT NULL,
  node_id TEXT,
  processing_time_ms INTEGER,
  resolution TEXT,
  embedding_json TEXT NOT NULL,
  image_embedding_json TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_vlm_semantic_camera_time ON vlm_semantic_cache(camera_id, timestamp_unix DESC);
CREATE INDEX IF NOT EXISTS idx_vlm_semantic_time ON vlm_semantic_cache(timestamp_unix DESC);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return err
	}
	// 兼容旧库：补充新列
	_, _ = s.db.Exec(`ALTER TABLE vlm_semantic_cache ADD COLUMN image_embedding_json TEXT`)
	_, _ = s.db.Exec(`ALTER TABLE vlm_semantic_cache ADD COLUMN stream_name TEXT`)
	return nil
}

func (s *Store) UpsertHistoryItems(items []HistoryItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
INSERT INTO vlm_semantic_cache
(task_id, timestamp_unix, camera_id, stream_name, thumbnail, result_text, node_id, processing_time_ms, resolution, embedding_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
  timestamp_unix=excluded.timestamp_unix,
  camera_id=excluded.camera_id,
  stream_name=excluded.stream_name,
  thumbnail=excluded.thumbnail,
  result_text=excluded.result_text,
  node_id=excluded.node_id,
  processing_time_ms=excluded.processing_time_ms,
  resolution=excluded.resolution,
  embedding_json=excluded.embedding_json,
  updated_at=excluded.updated_at;
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	var pendingImageTasks []struct {
		TaskID    string
		Thumbnail string
	}
	for _, it := range items {
		cameraID := strings.TrimSpace(it.CameraID)
		if cameraID == "" {
			cameraID = strings.TrimSpace(it.StreamName)
		}
		if cameraID == "" || strings.TrimSpace(it.TaskID) == "" {
			continue
		}
		ts := parseTimestamp(it.Timestamp)
		text := strings.TrimSpace(it.Result)
		if text == "" {
			text = "(空结果)"
		}
		// 使用 EmbedText（优先语义向量，回退到 hash）
		emb := EmbedText(text, s.vectorDim)
		embJSON, _ := json.Marshal(emb)

		if _, err = stmt.Exec(
			it.TaskID,
			ts.Unix(),
			cameraID,
			it.StreamName,
			it.Thumbnail,
			text,
			it.NodeID,
			it.ProcessingTime,
			it.Resolution,
			string(embJSON),
			now,
			now,
		); err != nil {
			return err
		}

		// Phase-3: 收集需要生成图像向量的任务
		if thumbnail := strings.TrimSpace(it.Thumbnail); thumbnail != "" {
			pendingImageTasks = append(pendingImageTasks, struct {
				TaskID    string
				Thumbnail string
			}{it.TaskID, thumbnail})
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	// 异步处理图像向量生成
	if len(pendingImageTasks) > 0 {
		go func(tasks []struct {
			TaskID    string
			Thumbnail string
		}) {
			for _, t := range tasks {
				if imgEmb, ok := RuntimeEmbedImage(t.Thumbnail); ok && len(imgEmb) > 0 {
					if err := s.UpsertImageEmbedding(t.TaskID, imgEmb); err != nil {
						log.Printf("[ImageEmbed] %s 存储失败: %v", t.TaskID, err)
					}
				}
			}
			log.Printf("[ImageEmbed] 批量处理完成，共 %d 条", len(tasks))
		}(pendingImageTasks)
	}
	return nil
}

func (s *Store) Search(query string, cameraID string, limit int) ([]SearchResult, error) {
if limit <= 0 || limit > 200 {
limit = 50
}
// 优先使用 runtimeEmbedText (调用 rivision-embed)
qEmb, ok := runtimeEmbedText(query, s.vectorDim)
if !ok || len(qEmb) == 0 {
log.Printf("[Search] runtimeEmbedText 失败，回退到 hashEmbed")
qEmb = hashEmbed(query, s.vectorDim)
} else {
log.Printf("[Search] 使用 runtimeEmbedText 成功，向量维度=%d", len(qEmb))
}

	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(cameraID) == "" || cameraID == "all" {
		rows, err = s.db.Query(`
SELECT task_id, timestamp_unix, camera_id, COALESCE(stream_name,'') as stream_name, thumbnail, result_text, node_id, processing_time_ms, resolution, embedding_json
FROM vlm_semantic_cache
ORDER BY timestamp_unix DESC
LIMIT 2000`)
	} else {
		rows, err = s.db.Query(`
SELECT task_id, timestamp_unix, camera_id, COALESCE(stream_name,'') as stream_name, thumbnail, result_text, node_id, processing_time_ms, resolution, embedding_json
FROM vlm_semantic_cache
WHERE camera_id = ?
ORDER BY timestamp_unix DESC
LIMIT 2000`, cameraID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]SearchResult, 0, limit)
	type scored struct {
		SearchResult
		score float64
	}
	all := make([]scored, 0, 256)

	for rows.Next() {
		var (
			taskID, camID, streamName, thumb, text, nodeID, resolution, embJSON string
			tsUnix                                                              int64
			procMs                                                              int64
		)
		if err := rows.Scan(&taskID, &tsUnix, &camID, &streamName, &thumb, &text, &nodeID, &procMs, &resolution, &embJSON); err != nil {
			return nil, err
		}
		var emb []float64
		if err := json.Unmarshal([]byte(embJSON), &emb); err != nil {
			continue
		}
		score := cosineSimilarity(qEmb, emb)
		all = append(all, scored{SearchResult: SearchResult{
			TaskID:         taskID,
			Timestamp:      time.Unix(tsUnix, 0),
			CameraID:       camID,
			StreamName:     streamName,
			Thumbnail:      thumb,
			Result:         text,
			NodeID:         nodeID,
			ProcessingTime: procMs,
			Resolution:     resolution,
			Score:          score,
		}, score: score})
	}

	// 先按分数排序筛选相关结果
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })
	// 保留分数最高的结果
	maxKeep := limit * 2
	if len(all) > maxKeep {
		all = all[:maxKeep]
	}
	// 最终按时间降序排列（最新在前）
	sort.Slice(all, func(i, j int) bool { return all[i].Timestamp.After(all[j].Timestamp) })
	for i := 0; i < len(all) && i < limit; i++ {
		results = append(results, all[i].SearchResult)
	}
	return results, nil
}

func parseTimestamp(v interface{}) time.Time {
	switch t := v.(type) {
	case string:
		if tt, err := time.Parse(time.RFC3339, t); err == nil {
			return tt
		}
		if tt, err := time.Parse("2006-01-02 15:04:05", t); err == nil {
			return tt
		}
	case float64:
		return time.Unix(int64(t), 0)
	case int64:
		return time.Unix(t, 0)
	}
	return time.Now()
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// updateTextEmbedding 异步更新文本语义向量
func (s *Store) updateTextEmbedding(taskID string, embedding []float64) error {
	if strings.TrimSpace(taskID) == "" || len(embedding) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	b, _ := json.Marshal(embedding)
	_, err := s.db.Exec(`UPDATE vlm_semantic_cache SET embedding_json=?, updated_at=? WHERE task_id=?`, string(b), time.Now().Unix(), taskID)
	return err
}

// UpsertImageEmbedding 为指定 task 写入图像向量（Phase-3）。
func (s *Store) UpsertImageEmbedding(taskID string, imageEmbedding []float64) error {
	if strings.TrimSpace(taskID) == "" || len(imageEmbedding) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	
	b, _ := json.Marshal(imageEmbedding)
	_, err := s.db.Exec(`UPDATE vlm_semantic_cache SET image_embedding_json=?, updated_at=? WHERE task_id=?`, string(b), time.Now().Unix(), taskID)
	return err
}

func (s *Store) Stats() (int64, error) {
	row := s.db.QueryRow(`SELECT COUNT(1) FROM vlm_semantic_cache`)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) String() string {
	return fmt.Sprintf("semantic_store(dim=%d)", s.vectorDim)
}

// BackfillImageEmbeddings 为缺少图像向量的记录生成向量
func (s *Store) BackfillImageEmbeddings(limit int) (int, error) {
log.Printf("[Backfill] 开始回填，限制 %d 条", limit)
rows, err := s.db.Query(`
SELECT task_id, thumbnail FROM vlm_semantic_cache 
WHERE (image_embedding_json IS NULL OR image_embedding_json = '') 
AND thumbnail IS NOT NULL AND thumbnail != ''
LIMIT ?`, limit)
if err != nil {
log.Printf("[Backfill] 查询失败: %v", err)
return 0, err
}
defer rows.Close()

var tasks []struct {
TaskID    string
Thumbnail string
}
for rows.Next() {
var t struct {
TaskID    string
Thumbnail string
}
if err := rows.Scan(&t.TaskID, &t.Thumbnail); err != nil {
continue
}
tasks = append(tasks, t)
}
log.Printf("[Backfill] 找到 %d 条需要处理的记录", len(tasks))

count := 0
for i, t := range tasks {
log.Printf("[Backfill] 处理 %d/%d: %s (thumbnail长度=%d)", i+1, len(tasks), t.TaskID, len(t.Thumbnail))
imgEmb, ok := RuntimeEmbedImage(t.Thumbnail)
if !ok {
log.Printf("[Backfill] %s RuntimeEmbedImage 返回 false", t.TaskID)
continue
}
if len(imgEmb) == 0 {
log.Printf("[Backfill] %s 向量为空", t.TaskID)
continue
}
if err := s.UpsertImageEmbedding(t.TaskID, imgEmb); err != nil {
log.Printf("[Backfill] %s 存储失败: %v", t.TaskID, err)
continue
}
count++
log.Printf("[Backfill] %s 成功 (%d维)", t.TaskID, len(imgEmb))
}
log.Printf("[Backfill] 完成，成功处理 %d 条", count)
return count, nil
}
