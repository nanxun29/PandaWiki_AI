ALTER TABLE nodes ADD COLUMN revision bigint NOT NULL DEFAULT 1;
ALTER TABLE conversations ADD COLUMN admin_user_id text NOT NULL DEFAULT '';
CREATE FUNCTION knowledge_agent_node_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.content IS DISTINCT FROM OLD.content OR NEW.name IS DISTINCT FROM OLD.name
 OR NEW.meta IS DISTINCT FROM OLD.meta OR NEW.nav_id IS DISTINCT FROM OLD.nav_id
 OR NEW.parent_id IS DISTINCT FROM OLD.parent_id OR NEW.position IS DISTINCT FROM OLD.position
 OR NEW.permissions IS DISTINCT FROM OLD.permissions OR NEW.type IS DISTINCT FROM OLD.type
 OR NEW.status IS DISTINCT FROM OLD.status THEN
  NEW.revision := OLD.revision + 1;
 ELSE NEW.revision := OLD.revision;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER knowledge_agent_node_revision BEFORE UPDATE ON nodes
FOR EACH ROW EXECUTE FUNCTION knowledge_agent_node_revision();

CREATE TABLE knowledge_agent_policies (
 kb_id text PRIMARY KEY REFERENCES knowledge_bases(id) ON DELETE CASCADE,
 mode text NOT NULL DEFAULT 'off' CHECK(mode IN ('off','manual','draft')),
 model_id text NOT NULL DEFAULT '', allow_model_transfer boolean NOT NULL DEFAULT false,
 user_ids jsonb NOT NULL DEFAULT '[]', folder_ids jsonb NOT NULL DEFAULT '[]',
 daily_calls integer NOT NULL DEFAULT 50 CHECK(daily_calls BETWEEN 1 AND 10000),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE knowledge_agent_conversation_states (
 conversation_id text PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
 kb_id text NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
 user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 watermark timestamptz NOT NULL DEFAULT '1970-01-01', watermark_id text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE knowledge_agent_tasks (
 id text PRIMARY KEY, kb_id text NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
 user_id text NOT NULL, conversation_id text NOT NULL DEFAULT '', kind text NOT NULL CHECK(kind IN ('extract','index','chat')),
 task_key text NOT NULL UNIQUE, status text NOT NULL CHECK(status IN ('pending','running','done','failed','cancelled','paused')),
 attempts integer NOT NULL DEFAULT 0, available_at timestamptz NOT NULL DEFAULT now(),
 lease_until timestamptz, lease_token text NOT NULL DEFAULT '', message_ids jsonb NOT NULL DEFAULT '[]',
 release_ids jsonb NOT NULL DEFAULT '[]', error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX knowledge_agent_active_conversation ON knowledge_agent_tasks(conversation_id)
WHERE kind='extract' AND status IN ('pending','running','paused');
CREATE INDEX knowledge_agent_due_tasks ON knowledge_agent_tasks(status, available_at);
CREATE TABLE knowledge_agent_changes (
 id text PRIMARY KEY, kb_id text NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
 user_id text NOT NULL, task_id text NOT NULL UNIQUE REFERENCES knowledge_agent_tasks(id), fingerprint text NOT NULL DEFAULT '',
 conversation_id text NOT NULL, model_id text NOT NULL, proposal jsonb NOT NULL,
 base_revision bigint NOT NULL DEFAULT 0, published_revision bigint NOT NULL DEFAULT 0, before text NOT NULL DEFAULT '', after text NOT NULL DEFAULT '',
 content_type text NOT NULL DEFAULT 'md', revision bigint NOT NULL DEFAULT 1,
 status text NOT NULL CHECK(status IN ('pending','published','rejected')),
 reviewer_id text NOT NULL DEFAULT '', reviewed_at timestamptz,
 document_id text NOT NULL DEFAULT '', release_id text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX knowledge_agent_inbox ON knowledge_agent_changes(kb_id,status,created_at);
CREATE UNIQUE INDEX knowledge_agent_pending_fingerprint ON knowledge_agent_changes(kb_id,fingerprint) WHERE status='pending' AND fingerprint <> '';
CREATE TABLE knowledge_agent_sources (
 id text PRIMARY KEY, change_id text NOT NULL REFERENCES knowledge_agent_changes(id) ON DELETE CASCADE,
 conversation_id text NOT NULL, message_id text NOT NULL, user_id text NOT NULL,
 UNIQUE(change_id,message_id)
);
CREATE TABLE knowledge_agent_audits (
 id text PRIMARY KEY, kb_id text NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
 user_id text NOT NULL, change_id text NOT NULL DEFAULT '', action text NOT NULL, result text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX knowledge_agent_audit_lookup ON knowledge_agent_audits(kb_id,created_at);
