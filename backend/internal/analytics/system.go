package analytics

import (
	"context"
	"endosoflow/internal/auth"
)

// System answers the system tab for Admins: email delivery, sign-ins, accounts and storage upkeep.
// Only the period applies; the sales filters do not.
func (s *Service) System(ctx context.Context, actor auth.Actor, f Filter) (*System, error) {
	if actor.Role != auth.RoleSystemAdmin {
		return nil, errForbidden
	}
	sc, err := s.scopeFor(actor, f, false)
	if err != nil {
		return nil, err
	}
	out := &System{Meta: s.meta(sc)}
	out.Notifications.FailedByType = []FailedByType{}
	err = s.read(ctx, func(q querier) error {
		if err := q.QueryRow(ctx, cteP+`
SELECT
	(SELECT count(*) FROM notification_logs WHERE status = 'pending')::int,
	count(*) FILTER (WHERE n.status = 'sent')::int,
	count(*) FILTER (WHERE n.status = 'failed')::int,
	count(*) FILTER (WHERE n.status = 'skipped')::int
FROM p LEFT JOIN notification_logs n ON `+inPeriod("n.created_at"), sc.args()...).Scan(
			&out.Notifications.Pending, &out.Notifications.Sent, &out.Notifications.Failed, &out.Notifications.Skipped); err != nil {
			return err
		}
		rows, err := q.Query(ctx, cteP+`
SELECT n.notification_type::text, count(*)::int
FROM notification_logs n CROSS JOIN p
WHERE n.status = 'failed' AND `+inPeriod("n.created_at")+`
GROUP BY n.notification_type ORDER BY count(*) DESC, n.notification_type::text`, sc.args()...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var ft FailedByType
			if err := rows.Scan(&ft.Type, &ft.Count); err != nil {
				rows.Close()
				return err
			}
			out.Notifications.FailedByType = append(out.Notifications.FailedByType, ft)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if err := q.QueryRow(ctx, cteP+`
SELECT
	count(*) FILTER (WHERE a.action_type = 'login_success')::int,
	count(*) FILTER (WHERE a.action_type = 'login_failed')::int,
	(SELECT count(*) FROM employees e WHERE e.locked_until > p.now_ts)::int
FROM p LEFT JOIN access_logs a ON a.account_type = 'employee' AND `+inPeriod("a.created_at")+`
GROUP BY p.now_ts`, sc.args()...).Scan(&out.Security.SuccessfulSignIns, &out.Security.FailedSignIns, &out.Security.LockedAccounts); err != nil {
			return err
		}
		if err := q.QueryRow(ctx, cteP+`
SELECT count(*) FILTER (WHERE is_active)::int, count(*) FILTER (WHERE is_active AND activated_at IS NULL)::int
FROM employees WHERE NOT is_system`, sc.args()...).Scan(
			&out.Accounts.ActiveEmployees, &out.Accounts.NotActivatedEmployees); err != nil {
			return err
		}
		return q.QueryRow(ctx, cteP+`
SELECT count(*)::int FROM storage_deletions WHERE completed_at IS NULL`, sc.args()...).Scan(
			&out.Storage.PendingDeletions)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
