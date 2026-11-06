package sales

import (
	"endosoflow/internal/auth"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestCanCancel(t *testing.T) {
	advisorID := uuid.New()
	own := &Sale{AdvisorID: advisorID}
	other := &Sale{AdvisorID: uuid.New()}
	advisor := auth.Actor{ID: advisorID, Role: auth.RoleSalesAdvisor}
	manager := auth.Actor{ID: uuid.New(), Role: auth.RoleSalesManager}
	admin := auth.Actor{ID: uuid.New(), Role: auth.RoleSystemAdmin}

	for task := 1; task <= 13; task++ {
		if got, want := canCancel(advisor, own, task), task <= 3; got != want {
			t.Errorf("advisor, own sale, task %d: got %v, want %v", task, got, want)
		}
		if canCancel(advisor, other, task) {
			t.Errorf("advisor, someone else's sale, task %d: must not cancel", task)
		}
		if got, want := canCancel(manager, own, task), task <= 6; got != want {
			t.Errorf("manager, task %d: got %v, want %v", task, got, want)
		}
		if !canCancel(admin, own, task) {
			t.Errorf("admin, task %d: must cancel", task)
		}
	}
	if canCancel(auth.Actor{ID: uuid.New(), Role: auth.Role("customer")}, own, 1) {
		t.Error("an unknown role must not cancel")
	}
}

func TestVisibleAppliesTheAdvisorScope(t *testing.T) {
	id := uuid.New()
	sale := &Sale{AdvisorID: id}
	if !visible(auth.Actor{ID: id, Role: auth.RoleSalesAdvisor}, sale) {
		t.Error("an advisor sees their own sale")
	}
	if visible(auth.Actor{ID: uuid.New(), Role: auth.RoleSalesAdvisor}, sale) {
		t.Error("an advisor must not see someone else's sale")
	}
	if !visible(auth.Actor{ID: uuid.New(), Role: auth.RoleSalesManager}, sale) || !visible(auth.Actor{ID: uuid.New(), Role: auth.RoleSystemAdmin}, sale) {
		t.Error("management sees every sale")
	}
	if visible(auth.Actor{ID: id, Role: auth.RoleSystemAdmin}, nil) {
		t.Error("a missing sale is never visible")
	}
}

func TestListFilterReadsAndValidatesTheQuery(t *testing.T) {
	advisor := uuid.New()
	req := httptest.NewRequest(http.MethodGet,
		"/sales?q=emba&state=active&currentTask=4&milestone=2&advisorId="+advisor.String()+
			"&customerType=embajada&temperature=hot&page=2&pageSize=50&sort=-customerName", nil)
	f, err := listFilter(req)
	if err != nil {
		t.Fatalf("valid query refused: %v", err)
	}
	if f.Search != "emba" || f.State != StateActive || f.CurrentTask != 4 || f.Milestone != 2 ||
		f.AdvisorID == nil || *f.AdvisorID != advisor || f.Page != 2 || f.PageSize != 50 || f.Sort != "-customerName" {
		t.Errorf("filter = %+v", f)
	}

	for _, bad := range []string{
		"state=deleted", "currentTask=14", "currentTask=0", "milestone=7", "advisorId=nope", "customerType=otro",
		"temperature=lukewarm", "page=0", "pageSize=abc", "sort=password", "modelId=1", "versionId=1",
	} {
		if _, err := listFilter(httptest.NewRequest(http.MethodGet, "/sales?"+bad, nil)); err == nil {
			t.Errorf("query %q must be refused", bad)
		}
	}
	if f, err := listFilter(httptest.NewRequest(http.MethodGet, "/sales", nil)); err != nil || f.Page != 0 || f.State != "" {
		t.Errorf("an empty query must give an empty filter: %+v, %v", f, err)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`50%_off\`); got != `50\%\_off\\` {
		t.Fatalf("escapeLike = %q", got)
	}
}

func TestOneOf(t *testing.T) {
	if !oneOf("hot", temperatures) || oneOf("tepid", temperatures) || oneOf("", temperatures) {
		t.Error("oneOf must match exactly")
	}
	if !oneOf("other", cancellationCategories) {
		t.Error("other is a cancellation category")
	}
}
