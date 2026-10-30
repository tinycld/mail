package mail

import (
	"fmt"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/groups"
)

// maxCoverageGroups bounds how many groups one domain may name. It is a sanity
// bound for the admin, not a query limit: groupMembers reads every group in
// one IN query.
const maxCoverageGroups = 100

// groupMembers maps each user in any of groupIDs to the groups (of groupIDs,
// in groupIDs order) they belong to. The order decides via_group: the first
// of a domain's groups a person is in is the one that gave the address.
func groupMembers(app core.App, groupIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(groupIDs) == 0 {
		return out, nil
	}
	ids := make([]any, len(groupIDs))
	for i, id := range groupIDs {
		ids[i] = id
	}
	var rows []struct {
		Group string `db:"group"`
		User  string `db:"user"`
	}
	err := app.DB().
		Select("group", "user").
		From("group_members").
		Where(dbx.In("group", ids...)).
		All(&rows)
	if err != nil {
		return nil, fmt.Errorf("list group members: %w", err)
	}
	in := map[string]map[string]bool{}
	for _, row := range rows {
		if in[row.User] == nil {
			in[row.User] = map[string]bool{}
		}
		in[row.User][row.Group] = true
	}
	for user, groups := range in {
		for _, id := range groupIDs {
			if groups[id] {
				out[user] = append(out[user], id)
				delete(groups, id)
			}
		}
	}
	return out, nil
}

// checkCoverageGroups refuses a groups domain that names no group, or too
// many. It runs on API requests only: when a group is deleted, PocketBase
// removes it from coverage_groups with a plain save, and that save must not
// be refused even when it removes the domain's last group.
func checkCoverageGroups(domain *core.Record) error {
	if domain.GetString("coverage") != coverageGroups {
		return nil
	}
	count := len(domain.GetStringSlice("coverage_groups"))
	if count == 0 {
		return validation.Errors{"coverage_groups": validation.NewError("coverage_groups_required", "Choose at least one group")}
	}
	if count > maxCoverageGroups {
		return validation.Errors{"coverage_groups": validation.NewError("coverage_groups_too_many",
			fmt.Sprintf("Choose at most %d groups", maxCoverageGroups))}
	}
	return nil
}

// registerGroupMembershipListener keeps rule addresses on group domains in
// step with group membership. Core delivers each join and leave after it
// commits — also the leaves a group delete or an offboard causes — and sends
// none at boot, which the boot reconcile covers. Core ignores a repeat of
// the same name, so every app's Register can call this.
func registerGroupMembershipListener() {
	groups.OnMembershipChange("mail.rule-addresses", onGroupMembershipChange)
}

// onGroupMembershipChange reconciles the rule addresses of the person who
// joined or left, through the app that saved the membership: the listener
// is registered once per process, so an app captured at registration can be
// another app. The reconcile error goes back to core, which logs it.
func onGroupMembershipChange(e groups.MembershipEvent) error {
	return reconcileRuleAddresses(e.App, ruleScope{UserID: e.UserID})
}
