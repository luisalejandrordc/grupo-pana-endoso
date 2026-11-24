// Package portal is the read-only API of the customer portal.
//
// It is deliberately thin. It owns no business data: it authorizes (a signed-in customer, and only that
// customer's own sales) and composes what the modules that do own the data expose for customers:
//
//   - the sale and its six milestones, from the sales tables and v_sale_milestones;
//   - the visible fields of the completed tasks, from tasks.CustomerView (an allowlist, default-deny);
//   - the downloadable files, from documents.CustomerDocuments, signed by documents.CustomerDownloadURL;
//   - the vehicle's categories and photos, from catalog.Presentation.
//
// Every query takes the customer from the access token, never from the request. A sale or a document that is
// someone else's answers exactly like one that does not exist.
package portal
