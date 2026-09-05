package server

import (
	"context"

	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
	"github.com/voocel/ainovel-cli/internal/server/qualityruntime"
)

func (s *Server) configuredQualityModel(ctx context.Context, projectID string) (qualitygate.ModelInvoker, error) {
	if s.cfg.QualityModel != nil { return s.cfg.QualityModel, nil }
	if !s.cfg.QualityConfigEnabled { return nil, nil }
	var cfg bootstrap.Config
	if projectID != "" {
		loaded, err := s.projects.LoadModelConfig(ctx, projectID, s.cfg.QualityConfigPath)
		if err != nil { return nil, err }
		cfg = loaded
	} else {
		loaded, err := bootstrap.LoadNovelForgeConfig("", s.cfg.Workspace, s.cfg.QualityConfigPath)
		if err != nil { return nil, err }
		cfg = loaded.Config
	}
	// Absent config permits project management; partial config must fail closed.
	if cfg.Provider == "" && cfg.ModelName == "" && len(cfg.Providers) == 0 && len(cfg.Roles) == 0 { return nil, nil }
	return qualityruntime.New(cfg)
}

func (s *Server) qualityConfigured(projectIDs ...string) bool {
	if s.cfg.QualityWriter != nil && s.cfg.QualityLibrarian != nil && s.cfg.QualityEditor != nil { return true }
	projectID := ""
	if len(projectIDs) > 0 { projectID = projectIDs[0] }
	model, err := s.configuredQualityModel(context.Background(), projectID)
	return err == nil && model != nil
}
