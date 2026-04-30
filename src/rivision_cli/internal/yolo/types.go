// Package yolo 提供 YOLO 目标检测功能
package yolo

import "time"

// Detection 检测结果
type Detection struct {
	ClassID    int       `json:"class_id"`
	ClassName  string    `json:"class_name"`
	Confidence float32   `json:"confidence"`
	BBox       BBox      `json:"bbox"`
	TrackID    int       `json:"track_id,omitempty"`
	// 速度信息（像素/秒）
	VelocityX  float32   `json:"velocity_x,omitempty"`
	VelocityY  float32   `json:"velocity_y,omitempty"`
}

// BBox 边界框
type BBox struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

// Width 获取宽度
func (b BBox) Width() int {
	return b.X2 - b.X1
}

// Height 获取高度
func (b BBox) Height() int {
	return b.Y2 - b.Y1
}

// Area 获取面积
func (b BBox) Area() int {
	return b.Width() * b.Height()
}

// Center 获取中心点
func (b BBox) Center() (int, int) {
	return (b.X1 + b.X2) / 2, (b.Y1 + b.Y2) / 2
}

// DetectionResult 检测结果集
type DetectionResult struct {
	CameraID        string      `json:"camera_id"`
	Timestamp       time.Time   `json:"timestamp"`
	FrameIndex      int64       `json:"frame_index"`
	Detections      []Detection `json:"detections"`
	InferenceTimeMs int64       `json:"inference_time_ms"`
	ImageWidth      int         `json:"image_width"`
	ImageHeight     int         `json:"image_height"`
	// 分布式推理附加信息
	NodeID      string `json:"node_id,omitempty"`      // 执行推理的节点ID
	TotalTimeMs int64  `json:"total_time_ms,omitempty"` // 总耗时（含网络传输）
}

// Count 获取检测数量
func (r *DetectionResult) Count() int {
	return len(r.Detections)
}

// CountByClass 按类别统计
func (r *DetectionResult) CountByClass(classID int) int {
	count := 0
	for _, d := range r.Detections {
		if d.ClassID == classID {
			count++
		}
	}
	return count
}

// FilterByConfidence 按置信度过滤
func (r *DetectionResult) FilterByConfidence(minConf float32) []Detection {
	var result []Detection
	for _, d := range r.Detections {
		if d.Confidence >= minConf {
			result = append(result, d)
		}
	}
	return result
}

// COCO 类别名称
var COCOClassNames = []string{
	"person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck", "boat",
	"traffic light", "fire hydrant", "stop sign", "parking meter", "bench", "bird", "cat",
	"dog", "horse", "sheep", "cow", "elephant", "bear", "zebra", "giraffe", "backpack",
	"umbrella", "handbag", "tie", "suitcase", "frisbee", "skis", "snowboard", "sports ball",
	"kite", "baseball bat", "baseball glove", "skateboard", "surfboard", "tennis racket",
	"bottle", "wine glass", "cup", "fork", "knife", "spoon", "bowl", "banana", "apple",
	"sandwich", "orange", "broccoli", "carrot", "hot dog", "pizza", "donut", "cake", "chair",
	"couch", "potted plant", "bed", "dining table", "toilet", "tv", "laptop", "mouse",
	"remote", "keyboard", "cell phone", "microwave", "oven", "toaster", "sink", "refrigerator",
	"book", "clock", "vase", "scissors", "teddy bear", "hair drier", "toothbrush",
}

// GetClassName 获取类别名称
func GetClassName(classID int) string {
	if classID >= 0 && classID < len(COCOClassNames) {
		return COCOClassNames[classID]
	}
	return "unknown"
}
