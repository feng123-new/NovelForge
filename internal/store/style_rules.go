package store

import (
	"os"

	"github.com/voocel/ainovel-cli/internal/domain"
)

// SaveStyleRules 保存写作风格规则。覆盖写入，最新弧的规则最准确。
func (s *Store) SaveStyleRules(rules domain.WritingStyleRules) error {
	return s.writeJSON("meta/style_rules.json", rules)
}

// LoadStyleRules 读取写作风格规则。无规则时返回 nil, nil。
func (s *Store) LoadStyleRules() (*domain.WritingStyleRules, error) {
	var rules domain.WritingStyleRules
	if err := s.readJSON("meta/style_rules.json", &rules); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &rules, nil
}
