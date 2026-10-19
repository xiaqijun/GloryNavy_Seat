-- +goose Up
ALTER TABLE feedback_images ADD COLUMN size_bytes bigint NOT NULL DEFAULT 0;
UPDATE feedback_images SET size_bytes=octet_length(content);
ALTER TABLE feedback_images DROP CONSTRAINT feedback_images_content_check;
ALTER TABLE feedback_images ADD CONSTRAINT feedback_images_content_check CHECK(size_bytes>0 AND (octet_length(content)=0 OR octet_length(content)=size_bytes));
CREATE TABLE feedback_image_chunks (
 image_id bigint NOT NULL REFERENCES feedback_images(id),
 ordinal bigint NOT NULL CHECK(ordinal>0),
 content bytea NOT NULL CHECK(octet_length(content) BETWEEN 1 AND 1048576),
 PRIMARY KEY(image_id,ordinal)
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM feedback_image_chunks) THEN
  RAISE EXCEPTION 'chunked screenshots exist; preserve attachments and deploy forward';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE feedback_image_chunks;
ALTER TABLE feedback_images DROP CONSTRAINT feedback_images_content_check;
ALTER TABLE feedback_images DROP COLUMN size_bytes;
ALTER TABLE feedback_images ADD CONSTRAINT feedback_images_content_check CHECK(octet_length(content) BETWEEN 1 AND 2000000);
