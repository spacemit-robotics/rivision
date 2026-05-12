// Package analytics 报表导出 - PDF/Excel
package analytics

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// ExportFormat 导出格式
type ExportFormat string

const (
	FormatCSV   ExportFormat = "csv"
	FormatJSON  ExportFormat = "json"
	FormatExcel ExportFormat = "xlsx"
	FormatPDF   ExportFormat = "pdf"
)

// ExportRequest 导出请求
type ExportRequest struct {
	ReportType string       `json:"report_type"` // traffic, alert, detection, heatmap
	Format     ExportFormat `json:"format"`
	StartTime  time.Time    `json:"start_time"`
	EndTime    time.Time    `json:"end_time"`
	CameraIDs  []string     `json:"camera_ids,omitempty"`
	Filters    map[string]string `json:"filters,omitempty"`
}

// ExportResult 导出结果
type ExportResult struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Data        []byte `json:"-"`
}

// Exporter 导出器
type Exporter struct {
	engine *Engine
}

// NewExporter 创建导出器
func NewExporter(engine *Engine) *Exporter {
	return &Exporter{engine: engine}
}

// Export 导出报表
func (e *Exporter) Export(req ExportRequest) (*ExportResult, error) {
	// 获取数据
	data, err := e.fetchReportData(req)
	if err != nil {
		return nil, err
	}

	// 根据格式导出
	var result *ExportResult
	switch req.Format {
	case FormatCSV:
		result, err = e.exportCSV(req, data)
	case FormatJSON:
		result, err = e.exportJSON(req, data)
	case FormatExcel:
		result, err = e.exportExcel(req, data)
	case FormatPDF:
		result, err = e.exportPDF(req, data)
	default:
		return nil, fmt.Errorf("unsupported format: %s", req.Format)
	}

	return result, err
}

// ReportData 报表数据
type ReportData struct {
	Title    string                   `json:"title"`
	Period   string                   `json:"period"`
	Summary  map[string]interface{}   `json:"summary"`
	Headers  []string                 `json:"headers"`
	Rows     [][]interface{}          `json:"rows"`
	Charts   []ChartData              `json:"charts,omitempty"`
}

// ChartData 图表数据
type ChartData struct {
	Type   string          `json:"type"` // line, bar, pie
	Title  string          `json:"title"`
	Labels []string        `json:"labels"`
	Data   [][]float64     `json:"data"`
	Series []string        `json:"series,omitempty"`
}

// fetchReportData 获取报表数据
func (e *Exporter) fetchReportData(req ExportRequest) (*ReportData, error) {
	switch req.ReportType {
	case "traffic":
		return e.fetchTrafficReport(req)
	case "alert":
		return e.fetchAlertReport(req)
	case "detection":
		return e.fetchDetectionReport(req)
	case "heatmap":
		return e.fetchHeatmapReport(req)
	default:
		return nil, fmt.Errorf("unknown report type: %s", req.ReportType)
	}
}

// fetchTrafficReport 获取客流报表
func (e *Exporter) fetchTrafficReport(req ExportRequest) (*ReportData, error) {
	// 获取客流统计数据（简化实现）
	stats := &TrafficStats{
		TotalIn:     1000,
		TotalOut:    950,
		PeakHour:    "14:00-15:00",
		AvgDaily:    200,
		DailyLabels: []string{},
		DailyIn:     []float64{},
		DailyOut:    []float64{},
	}

	data := &ReportData{
		Title:  "客流统计报表",
		Period: fmt.Sprintf("%s ~ %s", req.StartTime.Format("2006-01-02"), req.EndTime.Format("2006-01-02")),
		Summary: map[string]interface{}{
			"total_in":       stats.TotalIn,
			"total_out":      stats.TotalOut,
			"peak_hour":      stats.PeakHour,
			"avg_daily":      stats.AvgDaily,
		},
		Headers: []string{"日期", "进入", "离开", "净流入", "峰值时段", "峰值人数"},
		Rows:    make([][]interface{}, 0),
		Charts: []ChartData{
			{
				Type:   "line",
				Title:  "每日客流趋势",
				Labels: stats.DailyLabels,
				Data:   [][]float64{stats.DailyIn, stats.DailyOut},
				Series: []string{"进入", "离开"},
			},
			{
				Type:   "bar",
				Title:  "时段分布",
				Labels: []string{"0-4", "4-8", "8-12", "12-16", "16-20", "20-24"},
				Data:   [][]float64{stats.HourlyDist},
				Series: []string{"人数"},
			},
		},
	}

	// 填充行数据
	for i, label := range stats.DailyLabels {
		row := []interface{}{
			label,
			int(stats.DailyIn[i]),
			int(stats.DailyOut[i]),
			int(stats.DailyIn[i] - stats.DailyOut[i]),
			stats.DailyPeakHour[i],
			int(stats.DailyPeak[i]),
		}
		data.Rows = append(data.Rows, row)
	}

	return data, nil
}

// fetchAlertReport 获取告警报表
func (e *Exporter) fetchAlertReport(req ExportRequest) (*ReportData, error) {
	// 简化实现
	alerts := &AlertStats{
		Total:        100,
		ByLevel:      map[string]int{"high": 10, "medium": 30, "low": 60},
		Acknowledged: 80,
		Items:        []AlertItem{},
	}

	data := &ReportData{
		Title:  "告警统计报表",
		Period: fmt.Sprintf("%s ~ %s", req.StartTime.Format("2006-01-02"), req.EndTime.Format("2006-01-02")),
		Summary: map[string]interface{}{
			"total":        alerts.Total,
			"high":         alerts.ByLevel["high"],
			"medium":       alerts.ByLevel["medium"],
			"low":          alerts.ByLevel["low"],
			"acknowledged": alerts.Acknowledged,
		},
		Headers: []string{"时间", "级别", "类型", "摄像头", "描述", "状态"},
		Rows:    make([][]interface{}, 0),
		Charts: []ChartData{
			{
				Type:   "pie",
				Title:  "告警级别分布",
				Labels: []string{"高", "中", "低"},
				Data:   [][]float64{{float64(alerts.ByLevel["high"]), float64(alerts.ByLevel["medium"]), float64(alerts.ByLevel["low"])}},
			},
		},
	}

	for _, alert := range alerts.Items {
		row := []interface{}{
			alert.Timestamp.Format("2006-01-02 15:04:05"),
			alert.Level,
			alert.Type,
			alert.CameraName,
			alert.Message,
			alert.Status,
		}
		data.Rows = append(data.Rows, row)
	}

	return data, nil
}

// fetchDetectionReport 获取检测报表
func (e *Exporter) fetchDetectionReport(req ExportRequest) (*ReportData, error) {
	return &ReportData{
		Title:   "检测统计报表",
		Period:  fmt.Sprintf("%s ~ %s", req.StartTime.Format("2006-01-02"), req.EndTime.Format("2006-01-02")),
		Headers: []string{"摄像头", "检测总数", "人员", "车辆", "其他", "平均每小时"},
		Rows:    make([][]interface{}, 0),
	}, nil
}

// fetchHeatmapReport 获取热区报表
func (e *Exporter) fetchHeatmapReport(req ExportRequest) (*ReportData, error) {
	return &ReportData{
		Title:   "热区分析报表",
		Period:  fmt.Sprintf("%s ~ %s", req.StartTime.Format("2006-01-02"), req.EndTime.Format("2006-01-02")),
		Headers: []string{"摄像头", "区域", "访问次数", "平均停留(秒)", "峰值时段"},
		Rows:    make([][]interface{}, 0),
	}, nil
}

// exportCSV 导出CSV
func (e *Exporter) exportCSV(req ExportRequest, data *ReportData) (*ExportResult, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// 写入标题
	writer.Write([]string{data.Title})
	writer.Write([]string{data.Period})
	writer.Write([]string{})

	// 写入表头
	writer.Write(data.Headers)

	// 写入数据行
	for _, row := range data.Rows {
		strRow := make([]string, len(row))
		for i, v := range row {
			strRow[i] = fmt.Sprintf("%v", v)
		}
		writer.Write(strRow)
	}

	writer.Flush()

	return &ExportResult{
		Filename:    fmt.Sprintf("%s_%s.csv", req.ReportType, time.Now().Format("20060102")),
		ContentType: "text/csv; charset=utf-8",
		Size:        int64(buf.Len()),
		Data:        buf.Bytes(),
	}, nil
}

// exportJSON 导出JSON
func (e *Exporter) exportJSON(req ExportRequest, data *ReportData) (*ExportResult, error) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}

	return &ExportResult{
		Filename:    fmt.Sprintf("%s_%s.json", req.ReportType, time.Now().Format("20060102")),
		ContentType: "application/json",
		Size:        int64(len(jsonData)),
		Data:        jsonData,
	}, nil
}

// exportExcel 导出Excel
func (e *Exporter) exportExcel(req ExportRequest, data *ReportData) (*ExportResult, error) {
	// 简化实现：生成CSV格式但以xlsx扩展名
	// 生产环境应使用excelize库
	csvResult, err := e.exportCSV(req, data)
	if err != nil {
		return nil, err
	}

	return &ExportResult{
		Filename:    fmt.Sprintf("%s_%s.xlsx", req.ReportType, time.Now().Format("20060102")),
		ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		Size:        csvResult.Size,
		Data:        csvResult.Data,
	}, nil
}

// exportPDF 导出PDF
func (e *Exporter) exportPDF(req ExportRequest, data *ReportData) (*ExportResult, error) {
	// 简化实现：生成文本格式但以pdf扩展名
	// 生产环境应使用gofpdf或wkhtmltopdf
	var buf bytes.Buffer
	
	buf.WriteString(fmt.Sprintf("=== %s ===\n\n", data.Title))
	buf.WriteString(fmt.Sprintf("报表期间: %s\n\n", data.Period))
	
	// 摘要
	buf.WriteString("--- 摘要 ---\n")
	for k, v := range data.Summary {
		buf.WriteString(fmt.Sprintf("%s: %v\n", k, v))
	}
	buf.WriteString("\n")
	
	// 数据表
	buf.WriteString("--- 详细数据 ---\n")
	for _, h := range data.Headers {
		buf.WriteString(fmt.Sprintf("%s\t", h))
	}
	buf.WriteString("\n")
	
	for _, row := range data.Rows {
		for _, v := range row {
			buf.WriteString(fmt.Sprintf("%v\t", v))
		}
		buf.WriteString("\n")
	}

	return &ExportResult{
		Filename:    fmt.Sprintf("%s_%s.pdf", req.ReportType, time.Now().Format("20060102")),
		ContentType: "application/pdf",
		Size:        int64(buf.Len()),
		Data:        buf.Bytes(),
	}, nil
}

// ExportHandler HTTP处理器
func (e *Exporter) ExportHandler(w io.Writer, req ExportRequest) error {
	result, err := e.Export(req)
	if err != nil {
		return err
	}

	_, err = w.Write(result.Data)
	return err
}

// TrafficStats 客流统计（供fetchTrafficReport使用）
type TrafficStats struct {
	TotalIn       int64
	TotalOut      int64
	PeakHour      string
	AvgDaily      float64
	DailyLabels   []string
	DailyIn       []float64
	DailyOut      []float64
	DailyPeakHour []string
	DailyPeak     []float64
	HourlyDist    []float64
}

// AlertStats 告警统计
type AlertStats struct {
	Total        int64
	ByLevel      map[string]int
	Acknowledged int64
	Items        []AlertItem
}

// AlertItem 告警项
type AlertItem struct {
	Timestamp  time.Time
	Level      string
	Type       string
	CameraName string
	Message    string
	Status     string
}
