package idgen

import (
	"sync"
	"time"
)

// Snowflake 简易雪花ID生成器
type Snowflake struct {
	mu        sync.Mutex
	epoch     int64
	lastTime  int64
	sequence  int64
	machineID int64
}

// New 创建雪花ID生成器
func New(machineID int64) *Snowflake {
	return &Snowflake{
		epoch:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
		machineID: machineID & 0x3FF,
	}
}

// Next 生成下一个ID
func (s *Snowflake) Next() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli() - s.epoch
	if now == s.lastTime {
		s.sequence = (s.sequence + 1) & 0xFFF
		if s.sequence == 0 {
			for now <= s.lastTime {
				now = time.Now().UnixMilli() - s.epoch
			}
		}
	} else {
		s.sequence = 0
	}
	s.lastTime = now

	return (now << 22) | (s.machineID << 12) | s.sequence
}

// NextString 生成字符串ID
func (s *Snowflake) NextString() string {
	return formatInt64(s.Next())
}

func formatInt64(n int64) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
