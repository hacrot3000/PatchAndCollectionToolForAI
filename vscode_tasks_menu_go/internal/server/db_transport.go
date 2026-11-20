package server

import (
	"context"
	"errors"
	"fmt"

	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
	"bletonfc/vscode_tasks_menu/internal/sshtunnel"
)

func (s *Server) prepareDatabaseTransport(
	ctx context.Context,
	profile dbprofile.Profile,
) (string, int, sshtunnel.Metadata, func(), error) {
	switch profile.Transport {
	case "", dbprofile.TransportDirect:
		return profile.Host, profile.Port, sshtunnel.Metadata{}, nil, nil
	case dbprofile.TransportSSHTunnel:
		if s.SSHTunnels == nil {
			return "", 0, sshtunnel.Metadata{}, nil, errors.New("SSH tunnel manager is not configured")
		}
		sshProfiles, err := s.sshProfileStore()
		if err != nil {
			return "", 0, sshtunnel.Metadata{}, nil, err
		}
		sshProfile, err := sshProfiles.Get(profile.SSHProfileID)
		if errors.Is(err, sshprofile.ErrProfileNotFound) {
			return "", 0, sshtunnel.Metadata{}, nil, fmt.Errorf("SSH profile %q is not available", profile.SSHProfileID)
		}
		if err != nil {
			return "", 0, sshtunnel.Metadata{}, nil, err
		}
		tunnel, err := s.SSHTunnels.Open(ctx, sshProfile, profile.Host, profile.Port)
		if err != nil {
			return "", 0, sshtunnel.Metadata{}, nil, fmt.Errorf("open database SSH tunnel: %w", err)
		}
		cleanup := func() {
			err := s.SSHTunnels.CloseTunnel(tunnel.ID)
			if err != nil && !errors.Is(err, sshtunnel.ErrTunnelNotFound) && s.Log != nil {
				s.Log.Printf("database SSH tunnel cleanup warning tunnel=%s profile=%s: %v", tunnel.ID, profile.ID, err)
			}
		}
		return tunnel.LocalHost, tunnel.LocalPort, tunnel, cleanup, nil
	default:
		return "", 0, sshtunnel.Metadata{}, nil, fmt.Errorf("unsupported database transport %q", profile.Transport)
	}
}
