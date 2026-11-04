package gateway

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"

	"open-mihomo-gateway/internal/runtime"
)

func (m Manager) prepareSystemDNS(ctx context.Context, deps gatewayDeps) (runtime.SystemDNSSnapshot, error) {
	snapshot, err := m.localSystemDNS(deps).Prepare(ctx, m.cfg.Gateway.UpstreamInterface)
	if err != nil {
		return snapshot, err
	}
	backups, err := runtime.LoadSystemDNSBackups(m.paths.SystemDNSBackupFile)
	if err != nil {
		return snapshot, fmt.Errorf("read original Mac DNS backup: %w", err)
	}
	if original, ok := backups[snapshot.ServiceID]; ok {
		// A previous stop may have left our DNS setting installed. Preserve the
		// first backup for this service rather than backing up our own setting.
		snapshot.Servers = slices.Clone(original.Servers)
		snapshot.Resolvers = slices.Clone(original.Resolvers)
		snapshot.Domains = slices.Clone(original.Domains)
	}
	return snapshot, nil
}

func (m Manager) backupSystemDNS(snapshot runtime.SystemDNSSnapshot) error {
	backups, err := runtime.LoadSystemDNSBackups(m.paths.SystemDNSBackupFile)
	if err != nil {
		return fmt.Errorf("read original Mac DNS backup: %w", err)
	}
	if _, exists := backups[snapshot.ServiceID]; exists {
		return nil
	}
	backups[snapshot.ServiceID] = snapshot
	if err := runtime.SaveSystemDNSBackups(m.paths.SystemDNSBackupFile, backups); err != nil {
		return fmt.Errorf("save original Mac DNS backup: %w", err)
	}
	return nil
}

func warnSystemDNS(ctx context.Context, err error) {
	reportNotice(ctx, "system_dns_restore_failed")
	fmt.Fprintf(os.Stderr, "Warning: Mac system DNS recovery: %v; continuing gateway operation.\n", err)
}

// Restarts retain the backup even after a successful restore: a crash between
// stopping and reapplying DNS must not lose the original settings.
func (m Manager) restoreSystemDNSForRestart(ctx context.Context, deps gatewayDeps, state runtime.State) bool {
	if state.LocalSystemDNS == nil || !state.LocalSystemDNS.Owned {
		return false
	}
	if err := m.backupSystemDNS(*state.LocalSystemDNS); err != nil {
		warnSystemDNS(ctx, err)
	}
	ReportProgress(ctx, "restoring_system_dns")
	reapply, err := m.localSystemDNS(deps).Restore(ctx, *state.LocalSystemDNS)
	if err != nil {
		warnSystemDNS(ctx, err)
	}
	return reapply
}

// DNS restoration is independent of runtime cleanup. Failed entries survive
// stop/rollback in their own file and are retried by the next Stop, even when
// there is no gateway state left. A missing service is a completed entry.
func (m Manager) restoreSystemDNSBackups(ctx context.Context, deps gatewayDeps, state runtime.State) {
	backups, err := runtime.LoadSystemDNSBackups(m.paths.SystemDNSBackupFile)
	if err != nil {
		warnSystemDNS(ctx, err)
		// A damaged backup file must not stop cleanup or prevent an attempt
		// using the still-readable runtime snapshot. Do not overwrite that file.
		if state.LocalSystemDNS != nil && state.LocalSystemDNS.Owned {
			ReportProgress(ctx, "restoring_system_dns")
			if _, restoreErr := m.localSystemDNS(deps).Restore(ctx, *state.LocalSystemDNS); restoreErr != nil {
				warnSystemDNS(ctx, restoreErr)
			}
		}
		return
	}
	if snapshot := state.LocalSystemDNS; snapshot != nil && snapshot.Owned {
		if _, exists := backups[snapshot.ServiceID]; !exists {
			// Upgrade old runtime-only snapshots before removing runtime state.
			backups[snapshot.ServiceID] = *snapshot
			if err := runtime.SaveSystemDNSBackups(m.paths.SystemDNSBackupFile, backups); err != nil {
				warnSystemDNS(ctx, err)
			}
		}
	}
	if len(backups) == 0 {
		return
	}
	ReportProgress(ctx, "restoring_system_dns")
	ids := make([]string, 0, len(backups))
	for id := range backups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, err := m.localSystemDNS(deps).Restore(ctx, backups[id]); err != nil {
			warnSystemDNS(ctx, fmt.Errorf("service %s: %w", id, err))
			continue
		}
		delete(backups, id)
	}
	if err := runtime.SaveSystemDNSBackups(m.paths.SystemDNSBackupFile, backups); err != nil {
		warnSystemDNS(ctx, err)
	}
}
