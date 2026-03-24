package store

import (
	"fmt"
	"os"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// SaveReview 保存审阅结果。scope=chapter 写 reviews/{ch}.json，scope=global 写 reviews/{ch}-global.json。
func (s *Store) SaveReview(r domain.ReviewEntry) error {
	rel := fmt.Sprintf("reviews/%02d.json", r.Chapter)
	if r.Scope == "global" {
		rel = fmt.Sprintf("reviews/%02d-global.json", r.Chapter)
	}
	return s.writeJSON(rel, r)
}

// LoadReview 读取章节审阅结果。
func (s *Store) LoadReview(chapter int) (*domain.ReviewEntry, error) {
	var r domain.ReviewEntry
	if err := s.readJSON(fmt.Sprintf("reviews/%02d.json", chapter), &r); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// LoadLastReview 读取最近一次全局审阅。从 chapter 往前搜索。
func (s *Store) LoadLastReview(fromChapter int) (*domain.ReviewEntry, error) {
	for ch := fromChapter; ch >= 1; ch-- {
		var r domain.ReviewEntry
		if err := s.readJSON(fmt.Sprintf("reviews/%02d-global.json", ch), &r); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		return &r, nil
	}
	return nil, nil
}
