package mail

import (
	"context"
	"fmt"
	"strings"
)

// AllowList is the set of recipients a non-production environment may email. An entry is either a full address
// ("ana@example.com") or a whole domain ("@example.com"). Matching ignores case.
type AllowList struct {
	addresses map[string]bool
	domains   map[string]bool
}

// NewAllowList builds the list from its entries. Blank entries are ignored.
func NewAllowList(entries []string) AllowList {
	l := AllowList{addresses: map[string]bool{}, domains: map[string]bool{}}
	for _, e := range entries {
		e = strings.ToLower(strings.TrimSpace(e))
		switch {
		case e == "":
		case strings.HasPrefix(e, "@"):
			l.domains[e] = true
		default:
			l.addresses[e] = true
		}
	}
	return l
}

// Allows reports whether the address is on the list.
func (l AllowList) Allows(address string) bool {
	address = strings.ToLower(strings.TrimSpace(address))
	if l.addresses[address] {
		return true
	}
	at := strings.LastIndex(address, "@")
	return at > 0 && l.domains[address[at:]]
}

type restricted struct {
	next  Sender
	allow AllowList
}

// Restrict returns a sender that refuses (with ErrRecipientNotAllowed) every recipient that is not on the
// list, so a staging or development environment can never email a real customer by accident. It wraps the
// sender used by every module, so activation and reset emails are covered as well as notifications.
func Restrict(next Sender, allow AllowList) Sender { return restricted{next: next, allow: allow} }

func (r restricted) Send(ctx context.Context, msg Message) error {
	if !r.allow.Allows(msg.To) {
		return fmt.Errorf("%w: %s", ErrRecipientNotAllowed, msg.To)
	}
	return r.next.Send(ctx, msg)
}
