package sales_test

import (
	"archive/zip"
	"bytes"
	"endosoflow/internal/documents"
	"endosoflow/internal/tasks"
	"endosoflow/internal/testkit"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var pdfBytes = []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n%%EOF")

func xlsxBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range []string{"[Content_Types].xml", "xl/workbook.xml", "xl/worksheets/sheet1.xml"} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte("<x/>"))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (f *fx) put(who testkit.Employee, sale uuid.UUID, n int, slot string, data []byte, reason string) (*documents.File, bool, error) {
	return f.docs.Upload(f.Ctx, who.Actor(), documents.UploadInput{SaleID: sale, Number: n, Slot: slot, FileName: slot + ".bin", Data: data, Reason: reason})
}

// uploadRequired uploads a valid file to every required slot of a task, the way a user would.
func (f *fx) uploadRequired(sale uuid.UUID, n int) {
	f.T.Helper()
	d, err := f.tasks.Get(f.Ctx, f.advisor.Actor(), sale, n)
	if err != nil {
		f.T.Fatal(err)
	}
	for _, slot := range d.Documents {
		if !slot.Required || slot.File != nil {
			continue
		}
		data := pdfBytes
		if slot.AllowedFormat == "excel" {
			data = xlsxBytes(f.T)
		}
		if _, _, err := f.put(f.advisor, sale, n, slot.SlotCode, data, ""); err != nil {
			f.T.Fatalf("upload %s: %v", slot.SlotCode, err)
		}
	}
}

// completeWithUploads runs tasks 1 to n with real uploads instead of rows inserted behind the service's back.
func (f *fx) completeWithUploads(sale uuid.UUID, n int) {
	f.T.Helper()
	for i := 1; i <= n; i++ {
		if _, err := f.tasks.Start(f.Ctx, f.advisor.Actor(), sale, i); err != nil {
			f.T.Fatalf("start %d: %v", i, err)
		}
		if i != 7 {
			v := f.taskVersion(f.advisor, sale, i)
			if _, err := f.tasks.SaveData(f.Ctx, f.advisor.Actor(), sale, i, tasks.SaveInput{Version: &v, Data: []byte(f.validData(i))}); err != nil {
				f.T.Fatalf("save %d: %v", i, err)
			}
		}
		f.uploadRequired(sale, i)
		if _, err := f.tasks.Complete(f.Ctx, f.advisor.Actor(), sale, i); err != nil {
			f.T.Fatalf("complete %d: %v", i, err)
		}
	}
}

func TestUploadReplaceAndRemoveAFile(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	if _, err := f.tasks.Start(f.Ctx, f.advisor.Actor(), s.ID, 1); err != nil {
		t.Fatal(err)
	}

	first, replaced, err := f.put(f.advisor, s.ID, 1, "quotation_pdf", pdfBytes, "")
	if err != nil || replaced {
		t.Fatalf("first upload: replaced=%v err=%v", replaced, err)
	}
	if _, ok := f.store.Get(storageKey(t, f, first.ID)); !ok {
		t.Fatal("the file must be in the private documents store")
	}
	if first.MimeType != "application/pdf" || len(first.ChecksumSHA) != 64 {
		t.Fatalf("the type comes from the content and a checksum is kept: %+v", first)
	}

	oldKey := storageKey(t, f, first.ID)
	second, replaced, err := f.put(f.advisor, s.ID, 1, "quotation_pdf", append([]byte("%PDF-1.4\n"), []byte("second")...), "")
	if err != nil || !replaced {
		t.Fatalf("replacement: replaced=%v err=%v", replaced, err)
	}
	if _, ok := f.store.Get(oldKey); ok {
		t.Fatal("the replaced file must be deleted from storage once the change commits")
	}
	if _, ok := f.store.Get(storageKey(t, f, second.ID)); !ok {
		t.Fatal("the new file must be stored")
	}

	if err := f.docs.Remove(f.Ctx, f.advisor.Actor(), s.ID, 1, "quotation_pdf"); err != nil {
		t.Fatal(err)
	}
	if len(f.store.Keys()) != 0 {
		t.Fatalf("nothing should remain in storage: %v", f.store.Keys())
	}
	wantCode(t, f.docs.Remove(f.Ctx, f.advisor.Actor(), s.ID, 1, "quotation_pdf"), "DOCUMENT_NOT_FOUND")

	// With the file gone the task cannot be completed again, and the message names the missing document.
	v := f.taskVersion(f.advisor, s.ID, 1)
	if _, err := f.tasks.SaveData(f.Ctx, f.advisor.Actor(), s.ID, 1, tasks.SaveInput{Version: &v, Data: []byte(f.validData(1))}); err != nil {
		t.Fatal(err)
	}
	_, err = f.tasks.Complete(f.Ctx, f.advisor.Actor(), s.ID, 1)
	if fieldsOf(err)["documents.quotation_pdf"] != "DOCUMENT_REQUIRED" {
		t.Fatalf("a missing required file must block completion: %v", err)
	}
}

func storageKey(t *testing.T, f *fx, id uuid.UUID) string {
	t.Helper()
	var key string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT storage_key FROM documents WHERE id = $1`, id).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestFilesAreCheckedByContentSizeAndSlot(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	if _, err := f.tasks.Start(f.Ctx, f.advisor.Actor(), s.ID, 1); err != nil {
		t.Fatal(err)
	}
	_, _, err := f.put(f.advisor, s.ID, 1, "quotation_pdf", []byte("MZ\x90\x00 an executable renamed to .pdf"), "")
	wantCode(t, err, "DOCUMENT_TYPE_NOT_ALLOWED")
	_, _, err = f.put(f.advisor, s.ID, 1, "quotation_excel", pdfBytes, "")
	wantCode(t, err, "DOCUMENT_TYPE_NOT_ALLOWED")
	_, _, err = f.put(f.advisor, s.ID, 1, "quotation_pdf", append(pdfBytes, make([]byte, documents.MaxBytes)...), "")
	wantCode(t, err, "DOCUMENT_TOO_LARGE")
	_, _, err = f.put(f.advisor, s.ID, 1, "quotation_pdf", nil, "")
	wantCode(t, err, "DOCUMENT_REQUIRED")
	_, _, err = f.put(f.advisor, s.ID, 1, "signer_identity_scan", pdfBytes, "") // a slot of task 2
	wantCode(t, err, "DOCUMENT_SLOT_NOT_FOUND")
	if len(f.store.Keys()) != 0 {
		t.Fatalf("a refused file must never reach storage: %v", f.store.Keys())
	}
}

func TestUploadsNeedTheTaskInProgressAndTheRightPerson(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	_, _, err := f.put(f.advisor, s.ID, 1, "quotation_pdf", pdfBytes, "")
	wantCode(t, err, "TASK_NOT_IN_PROGRESS")

	if _, err := f.tasks.Start(f.Ctx, f.advisor.Actor(), s.ID, 1); err != nil {
		t.Fatal(err)
	}
	// An Advisor who does not own the sale cannot even see it; a Manager can act on any sale.
	_, _, err = f.put(f.other, s.ID, 1, "quotation_pdf", pdfBytes, "")
	wantCode(t, err, "SALE_NOT_FOUND")
	if _, _, err := f.put(f.manager, s.ID, 1, "quotation_pdf", pdfBytes, ""); err != nil {
		t.Fatalf("a manager may upload: %v", err)
	}
	_, err = f.docs.List(f.Ctx, f.other.Actor(), s.ID)
	wantCode(t, err, "SALE_NOT_FOUND")

	if _, err := f.sales.Cancel(f.Ctx, f.manager.Actor(), s.ID, "customer_withdrew", "Desistió"); err != nil {
		t.Fatal(err)
	}
	_, _, err = f.put(f.advisor, s.ID, 1, "quotation_pdf", pdfBytes, "")
	wantCode(t, err, "SALE_NOT_ACTIVE")
	wantCode(t, f.docs.Remove(f.Ctx, f.manager.Actor(), s.ID, 1, "quotation_pdf"), "SALE_NOT_ACTIVE")
}

func TestCompletedTasksOnlyAllowAReasonedReplacementByManagement(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	f.completeWithUploads(s.ID, 1)

	_, _, err := f.put(f.advisor, s.ID, 1, "quotation_pdf", pdfBytes, "quiero cambiarlo")
	wantCode(t, err, "FORBIDDEN")
	_, _, err = f.put(f.manager, s.ID, 1, "quotation_pdf", pdfBytes, "  ")
	wantCode(t, err, "REASON_REQUIRED")
	file, replaced, err := f.put(f.manager, s.ID, 1, "quotation_pdf", pdfBytes, "Cotización corregida")
	if err != nil || !replaced || file == nil {
		t.Fatalf("a reasoned replacement by a manager: replaced=%v err=%v", replaced, err)
	}
	// A completed task is never left without a required file.
	wantCode(t, f.docs.Remove(f.Ctx, f.manager.Actor(), s.ID, 1, "quotation_pdf"), "TASK_NOT_IN_PROGRESS")

	var reason string
	if err := f.Pool.QueryRow(f.Ctx, `SELECT coalesce(reason, '') FROM audit_logs WHERE entity_id = $1 AND action = 'REPLACED'`, file.ID).Scan(&reason); err != nil || reason != "Cotización corregida" {
		t.Fatalf("the replacement and its reason are audited: %q %v", reason, err)
	}
}

func TestSignedDownloadsAreShortLivedAndScoped(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	if _, err := f.tasks.Start(f.Ctx, f.advisor.Actor(), s.ID, 1); err != nil {
		t.Fatal(err)
	}
	file, _, err := f.put(f.advisor, s.ID, 1, "quotation_pdf", pdfBytes, "")
	if err != nil {
		t.Fatal(err)
	}
	dl, err := f.docs.DownloadURL(f.Ctx, f.advisor.Actor(), file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dl.URL, "ttl=5m0s") || dl.FileName != file.FileName || dl.ExpiresAt.Before(f.clock.Now()) || dl.ExpiresAt.After(f.clock.Now().Add(6*time.Minute)) {
		t.Fatalf("the address is short-lived and carries the original name: %+v", dl)
	}
	_, err = f.docs.DownloadURL(f.Ctx, f.other.Actor(), file.ID)
	wantCode(t, err, "DOCUMENT_NOT_FOUND")
	_, err = f.docs.DownloadURL(f.Ctx, f.manager.Actor(), uuid.New())
	wantCode(t, err, "DOCUMENT_NOT_FOUND")
}

func TestAllThirteenTasksCompleteWithRealUploads(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	f.completeWithUploads(s.ID, 13)

	got, err := f.sales.Get(f.Ctx, f.advisor.Actor(), s.ID)
	if err != nil || got.State != "completed" {
		t.Fatalf("the sale is completed after task 13: %v %+v", err, got)
	}
	list, err := f.docs.List(f.Ctx, f.advisor.Actor(), s.ID)
	if err != nil {
		t.Fatal(err)
	}
	filled := 0
	for _, slot := range list.Items {
		if slot.File != nil {
			filled++
		}
		if slot.Required && slot.File == nil {
			t.Fatalf("required slot %s is empty after the sale was completed", slot.SlotCode)
		}
	}
	if filled != len(f.store.Keys()) {
		t.Fatalf("every file in a slot is in storage and nothing else: %d slots, %d objects", filled, len(f.store.Keys()))
	}

	// A correction on a completed sale is still possible for management, with a reason (S5, T5).
	if _, _, err := f.put(f.manager, s.ID, 1, "quotation_pdf", pdfBytes, "Versión final"); err != nil {
		t.Fatalf("replacing a file on a completed sale: %v", err)
	}
}

func TestDeletingASaleRemovesItsFilesFromStorage(t *testing.T) {
	f := newFx(t)
	s := f.newSale(f.advisor)
	f.completeWithUploads(s.ID, 2)
	if len(f.store.Keys()) < 3 {
		t.Fatalf("expected the uploaded files in storage, got %v", f.store.Keys())
	}
	if err := f.sales.Delete(f.Ctx, f.admin.Actor(), s.ID, "Venta de prueba", s.Code); err != nil {
		t.Fatal(err)
	}
	if len(f.store.Keys()) != 0 {
		t.Fatalf("every file of a deleted sale must leave storage: %v", f.store.Keys())
	}
	var n int
	_ = f.Pool.QueryRow(f.Ctx, `SELECT count(*) FROM documents WHERE sale_id = $1`, s.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("document rows must be gone: %d", n)
	}
}
