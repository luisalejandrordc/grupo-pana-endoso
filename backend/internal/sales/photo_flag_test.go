package sales_test

import (
	"endosoflow/internal/sales"
	"testing"
)

// The screens ask for an advisor's picture only when it exists, so the sale (detail and list) must say so.
func TestSalesTellWhetherTheAdvisorHasAPicture(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	if s.Advisor.HasPhoto {
		t.Fatal("a new sale's advisor has no picture yet")
	}
	list := func() sales.ListItem {
		got, err := f.sales.List(f.Ctx, f.manager.Actor(), sales.ListFilter{Search: s.Code, Page: 1, PageSize: 5})
		if err != nil || len(got.Items) != 1 {
			t.Fatalf("list: %v %+v", err, got)
		}
		return got.Items[0]
	}
	if list().AdvisorHasPhoto {
		t.Fatal("the list says the advisor has a picture, but they have none")
	}

	if _, err := f.Pool.Exec(f.Ctx, `UPDATE employees SET profile_picture_key = 'employees/x/p.png', profile_picture_mime = 'image/png',
		profile_picture_size = 10 WHERE id = $1`, f.advisor.ID); err != nil {
		t.Fatal(err)
	}
	got, err := f.sales.Get(f.Ctx, f.manager.Actor(), s.ID)
	if err != nil || !got.Advisor.HasPhoto {
		t.Fatalf("the detail must say the advisor has a picture: %v %+v", err, got.Advisor)
	}
	if !list().AdvisorHasPhoto {
		t.Fatal("the list must say the advisor has a picture")
	}
}
