-- +goose Up
-- Possible duplicates: an incoming document is compared with the others of
-- its administration that share a supplier or a file. sha256 is the sum of
-- an attachment's contents, worked out on upload; older attachments get it
-- in the background, which finds them through attachments_unhashed_idx.
ALTER TABLE attachments ADD COLUMN sha256 text NOT NULL DEFAULT '';
CREATE INDEX attachments_sha256_idx ON attachments (administration_id, sha256) WHERE sha256 <> '';
CREATE INDEX attachments_unhashed_idx ON attachments (id) WHERE sha256 = '';
CREATE INDEX purchase_documents_admin_contact_idx ON purchase_documents (administration_id, contact_id);
CREATE INDEX general_documents_admin_contact_idx ON general_documents (administration_id, contact_id);
CREATE INDEX typeless_documents_admin_contact_idx ON typeless_documents (administration_id, contact_id);

-- +goose Down
DROP INDEX typeless_documents_admin_contact_idx;
DROP INDEX general_documents_admin_contact_idx;
DROP INDEX purchase_documents_admin_contact_idx;
DROP INDEX attachments_unhashed_idx;
DROP INDEX attachments_sha256_idx;
ALTER TABLE attachments DROP COLUMN sha256;
