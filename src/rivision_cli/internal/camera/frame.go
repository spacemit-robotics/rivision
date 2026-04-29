package camera

import (
	"time"
)

// Frame 视频帧
type Frame struct {
	CameraID  string    `json:"camera_id"`
	Timestamp time.Time `json:"timestamp"`
	Index     int64     `json:"index"`
	Data      []byte    `json:"-"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Format    string    `json:"format"` // jpeg, png, raw
}

// IsValid 检查帧是否有效
func (f *Frame) IsValid() bool {
	return f != nil && len(f.Data) > 0
}

// Size 获取数据大小
func (f *Frame) Size() int {
	if f == nil {
		return 0
	}
	return len(f.Data)
}

// Clone 克隆帧
func (f *Frame) Clone() *Frame {
	if f == nil {
		return nil
	}

	clone := &Frame{
		CameraID:  f.CameraID,
		Timestamp: f.Timestamp,
		Index:     f.Index,
		Width:     f.Width,
		Height:    f.Height,
		Format:    f.Format,
	}

	if len(f.Data) > 0 {
		clone.Data = make([]byte, len(f.Data))
		copy(clone.Data, f.Data)
	}

	return clone
}
