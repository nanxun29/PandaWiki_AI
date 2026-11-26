-- Operator-only rollback: export AI audit and proposals before dropping these tables.
DROP TABLE knowledge_agent_sources, knowledge_agent_changes, knowledge_agent_tasks,
 knowledge_agent_audits, knowledge_agent_conversation_states, knowledge_agent_policies;
DROP TRIGGER knowledge_agent_node_revision ON nodes;
DROP FUNCTION knowledge_agent_node_revision();
ALTER TABLE nodes DROP COLUMN revision;
ALTER TABLE conversations DROP COLUMN admin_user_id;
