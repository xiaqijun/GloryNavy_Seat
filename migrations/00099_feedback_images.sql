-- +goose Up
ALTER TABLE feedback_messages ADD CONSTRAINT feedback_messages_ticket_id_id_key UNIQUE(ticket_id,id);
CREATE TABLE feedback_images (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 ticket_id bigint NOT NULL REFERENCES feedback_tickets(id),
 message_id bigint,
 ordinal smallint NOT NULL CHECK(ordinal BETWEEN 1 AND 3),
 mime text NOT NULL CHECK(mime IN ('image/png','image/jpeg')),
 content bytea NOT NULL CHECK(octet_length(content) BETWEEN 1 AND 2000000),
 digest bytea NOT NULL CHECK(octet_length(digest)=32),
 FOREIGN KEY(ticket_id,message_id) REFERENCES feedback_messages(ticket_id,id),
 UNIQUE NULLS NOT DISTINCT(ticket_id,message_id,ordinal)
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM feedback_images) THEN
  RAISE EXCEPTION 'feedback screenshots exist; preserve attachments before rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE feedback_images;
ALTER TABLE feedback_messages DROP CONSTRAINT feedback_messages_ticket_id_id_key;
