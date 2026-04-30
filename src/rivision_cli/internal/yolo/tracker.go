// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package yolo

import (
	"sync"
)

// Track 跟踪对象
type Track struct {
	ID              int
	ClassID         int
	ClassName       string
	BBox            BBox
	Confidence      float32
	Age             int
	Hits            int
	TimeSinceUpdate int
	// 增强字段：速度和预测
	VelocityX       float32   // X方向速度 (像素/秒)
	VelocityY       float32   // Y方向速度 (像素/秒)
	LastUpdateTime  float64   // 最后更新时间戳
	Estimate        BBox      // Kalman预测位置
}

// Tracker ByteTrack 跟踪器
type Tracker struct {
	mu           sync.Mutex
	tracks       map[int]*Track
	nextID       int
	trackThresh  float64
	matchThresh  float64
	trackBuffer  int
}

// NewTracker 创建跟踪器
func NewTracker(trackThresh, matchThresh float64, trackBuffer int) *Tracker {
	return &Tracker{
		tracks:      make(map[int]*Track),
		nextID:      1,
		trackThresh: trackThresh,
		matchThresh: matchThresh,
		trackBuffer: trackBuffer,
	}
}

// UpdateWithTime 更新跟踪（带时间戳，用于计算速度）
func (t *Tracker) UpdateWithTime(detections []Detection, frameTime float64) []Detection {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make([]Detection, len(detections))
	copy(result, detections)

	// 为每个检测分配跟踪 ID
	for i := range result {
		matched := false
		var bestTrack *Track
		bestIoU := 0.0
		
		// 找到最佳匹配的track
		for _, track := range t.tracks {
			if track.ClassID == result[i].ClassID {
				iou := t.calculateIoU(track.BBox, result[i].BBox)
				if iou > t.matchThresh && iou > bestIoU {
					bestIoU = iou
					bestTrack = track
					matched = true
				}
			}
		}
		
		if matched && bestTrack != nil {
			// 计算速度 (像素/秒)
			dt := frameTime - bestTrack.LastUpdateTime
			if dt > 0 && dt < 2.0 { // 只在合理时间间隔内计算速度
				newCenterX := float32(result[i].BBox.X1+result[i].BBox.X2) / 2
				newCenterY := float32(result[i].BBox.Y1+result[i].BBox.Y2) / 2
				oldCenterX := float32(bestTrack.BBox.X1+bestTrack.BBox.X2) / 2
				oldCenterY := float32(bestTrack.BBox.Y1+bestTrack.BBox.Y2) / 2
				
				// 平滑速度更新 (指数移动平均)
				alpha := float32(0.3)
				newVx := (newCenterX - oldCenterX) / float32(dt)
				newVy := (newCenterY - oldCenterY) / float32(dt)
				bestTrack.VelocityX = alpha*newVx + (1-alpha)*bestTrack.VelocityX
				bestTrack.VelocityY = alpha*newVy + (1-alpha)*bestTrack.VelocityY
			}
			
			// 更新track
			result[i].TrackID = bestTrack.ID
			result[i].VelocityX = bestTrack.VelocityX
			result[i].VelocityY = bestTrack.VelocityY
			bestTrack.BBox = result[i].BBox
			bestTrack.Confidence = result[i].Confidence
			bestTrack.Hits++
			bestTrack.TimeSinceUpdate = 0
			bestTrack.LastUpdateTime = frameTime
		} else {
			// 创建新跟踪
			trackID := t.nextID
			t.nextID++
			result[i].TrackID = trackID
			result[i].VelocityX = 0
			result[i].VelocityY = 0
			t.tracks[trackID] = &Track{
				ID:             trackID,
				ClassID:        result[i].ClassID,
				ClassName:      result[i].ClassName,
				BBox:           result[i].BBox,
				Confidence:     result[i].Confidence,
				Age:            1,
				Hits:           1,
				LastUpdateTime: frameTime,
			}
		}
	}

	// 更新未匹配的跟踪
	for id, track := range t.tracks {
		track.TimeSinceUpdate++
		if track.TimeSinceUpdate > t.trackBuffer {
			delete(t.tracks, id)
		}
	}

	return result
}

// Update 更新跟踪（兼容旧接口）
func (t *Tracker) Update(detections []Detection) []Detection {
	return t.UpdateWithTime(detections, 0)
}

// calculateIoU 计算 IoU
func (t *Tracker) calculateIoU(a, b BBox) float64 {
	// 计算交集
	x1 := max(a.X1, b.X1)
	y1 := max(a.Y1, b.Y1)
	x2 := min(a.X2, b.X2)
	y2 := min(a.Y2, b.Y2)

	if x2 <= x1 || y2 <= y1 {
		return 0
	}

	intersection := float64((x2 - x1) * (y2 - y1))
	areaA := float64(a.Area())
	areaB := float64(b.Area())
	union := areaA + areaB - intersection

	if union <= 0 {
		return 0
	}

	return intersection / union
}

// GetTracks 获取所有跟踪
func (t *Tracker) GetTracks() []*Track {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make([]*Track, 0, len(t.tracks))
	for _, track := range t.tracks {
		result = append(result, track)
	}
	return result
}

// Reset 重置跟踪器
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.tracks = make(map[int]*Track)
	t.nextID = 1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
