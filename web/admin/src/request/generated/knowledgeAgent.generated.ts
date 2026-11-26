/* eslint-disable */
/* tslint:disable */
// @ts-nocheck
/*
 * ---------------------------------------------------------------
 * ## THIS FILE WAS GENERATED VIA SWAGGER-TYPESCRIPT-API        ##
 * ##                                                           ##
 * ## AUTHOR: acacode                                           ##
 * ## SOURCE: https://github.com/acacode/swagger-typescript-api ##
 * ---------------------------------------------------------------
 */

export enum SchemaRoleType {
  Assistant = "assistant",
  User = "user",
  System = "system",
  Tool = "tool",
}

export enum GithubComChaitinPandaWikiDomainModelProvider {
  ModelProviderBrandBaiZhiCloud = "BaiZhiCloud",
  ModelProviderBaiZhiCloudModelStore = "BaiZhiCloudModelStore",
}

export enum DomainScoreType {
  Like = 1,
  DisLike = -1,
}

export interface DomainConversationMessage {
  app_id?: string;
  completion_tokens?: number;
  content?: string;
  conversation_id?: string;
  created_at?: string;
  id?: string;
  image_paths?: string[];
  /** feedbackinfo */
  info?: DomainFeedBackInfo;
  kb_id?: string;
  model?: string;
  /** parent_id */
  parent_id?: string;
  prompt_tokens?: number;
  /** model */
  provider?: GithubComChaitinPandaWikiDomainModelProvider;
  /** stats */
  remote_ip?: string;
  role?: SchemaRoleType;
  total_tokens?: number;
}

export interface DomainFeedBackInfo {
  feedback_content?: string;
  feedback_type?: string;
  score?: DomainScoreType;
}

export interface DomainKnowledgeAudit {
  action?: string;
  change_id?: string;
  created_at?: string;
  id?: string;
  kb_id?: string;
  result?: string;
  user_id?: string;
}

export interface DomainKnowledgeChange {
  after?: string;
  base_revision?: number;
  before?: string;
  content_type?: string;
  conversation_id?: string;
  created_at?: string;
  document_id?: string;
  id?: string;
  kb_id?: string;
  model_id?: string;
  proposal?: DomainKnowledgeExtraction;
  published_revision?: number;
  release_id?: string;
  reviewed_at?: string;
  reviewer_id?: string;
  revision?: number;
  status?: string;
  task_id?: string;
  updated_at?: string;
  user_id?: string;
}

export interface DomainKnowledgeChatReq {
  conversation_id?: string;
  idempotency_key?: string;
  kb_id?: string;
  message?: string;
}

export interface DomainKnowledgeExtraction {
  action?: string;
  content?: string;
  knowledge_type?: string;
  nav_id?: string;
  new_text?: string;
  old_text?: string;
  parent_id?: string;
  reason?: string;
  risk_level?: string;
  should_save?: boolean;
  source_message_ids?: string[];
  summary?: string;
  target_document_id?: string;
  title?: string;
  verification_status?: string;
}

export interface DomainKnowledgePolicy {
  allow_model_transfer?: boolean;
  daily_calls?: number;
  folder_ids?: string[];
  kb_id?: string;
  mode?: string;
  model_id?: string;
  updated_at?: string;
  user_ids?: string[];
}

export interface DomainKnowledgeReviewReq {
  after?: string;
  id?: string;
  idempotency_key?: string;
  kb_id?: string;
  nav_id?: string;
  parent_id?: string;
  revision?: number;
  target_document_id?: string;
  title?: string;
}

export interface DomainKnowledgeTask {
  attempts?: number;
  available_at?: string;
  conversation_id?: string;
  created_at?: string;
  error?: string;
  id?: string;
  kb_id?: string;
  key?: string;
  kind?: string;
  lease_until?: string;
  message_ids?: string[];
  release_ids?: string[];
  status?: string;
  updated_at?: string;
  user_id?: string;
}

export interface DomainPWResponse {
  code?: number;
  data?: any;
  message?: string;
  success?: boolean;
}

export namespace Api {
  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentAuditsList
   * @summary 获取 AI 操作审计
   * @request GET:/api/v1/knowledge-agent/audits
   * @secure
   */
  export namespace V1KnowledgeAgentAuditsList {
    export type RequestParams = {};
    export type RequestQuery = {
      /** 知识库 ID */
      kb_id: string;
    };
    export type RequestBody = never;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse & {
      data?: DomainKnowledgeAudit[];
    };
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentChangesList
   * @summary 获取知识收件箱
   * @request GET:/api/v1/knowledge-agent/changes
   * @secure
   */
  export namespace V1KnowledgeAgentChangesList {
    export type RequestParams = {};
    export type RequestQuery = {
      /** 知识库 ID */
      kb_id: string;
      /** 状态 */
      status?: string;
    };
    export type RequestBody = never;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse & {
      data?: DomainKnowledgeChange[];
    };
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentChangesRegenerateCreate
   * @summary 重新提取知识建议
   * @request POST:/api/v1/knowledge-agent/changes/regenerate
   * @secure
   */
  export namespace V1KnowledgeAgentChangesRegenerateCreate {
    export type RequestParams = {};
    export type RequestQuery = {};
    export type RequestBody = DomainKnowledgeReviewReq;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse & {
      data?: DomainKnowledgeTask;
    };
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentChangesRollbackCreate
   * @summary 创建回滚建议
   * @request POST:/api/v1/knowledge-agent/changes/rollback
   * @secure
   */
  export namespace V1KnowledgeAgentChangesRollbackCreate {
    export type RequestParams = {};
    export type RequestQuery = {};
    export type RequestBody = DomainKnowledgeReviewReq;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse & {
      data?: DomainKnowledgeChange;
    };
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentChatCreate
   * @summary 后台知识助手流式聊天
   * @request POST:/api/v1/knowledge-agent/chat
   * @secure
   */
  export namespace V1KnowledgeAgentChatCreate {
    export type RequestParams = {};
    export type RequestQuery = {};
    export type RequestBody = DomainKnowledgeChatReq;
    export type RequestHeaders = {};
    export type ResponseBody = string;
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentExtractCreate
   * @summary 提交聊天知识整理任务
   * @request POST:/api/v1/knowledge-agent/extract
   * @secure
   */
  export namespace V1KnowledgeAgentExtractCreate {
    export type RequestParams = {};
    export type RequestQuery = {};
    export type RequestBody = DomainKnowledgeChatReq;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse & {
      data?: DomainKnowledgeTask;
    };
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentMessagesList
   * @summary 获取私有助手会话消息
   * @request GET:/api/v1/knowledge-agent/messages
   * @secure
   */
  export namespace V1KnowledgeAgentMessagesList {
    export type RequestParams = {};
    export type RequestQuery = {
      /** 知识库 ID */
      kb_id: string;
      /** 会话 ID */
      conversation_id: string;
    };
    export type RequestBody = never;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse & {
      data?: DomainConversationMessage[];
    };
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentPolicyList
   * @summary 获取知识管理策略
   * @request GET:/api/v1/knowledge-agent/policy
   * @secure
   */
  export namespace V1KnowledgeAgentPolicyList {
    export type RequestParams = {};
    export type RequestQuery = {
      /** 知识库 ID */
      kb_id: string;
    };
    export type RequestBody = never;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse;
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentPolicyUpdate
   * @summary 更新知识管理策略
   * @request PUT:/api/v1/knowledge-agent/policy
   * @secure
   */
  export namespace V1KnowledgeAgentPolicyUpdate {
    export type RequestParams = {};
    export type RequestQuery = {};
    export type RequestBody = DomainKnowledgePolicy;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse;
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentReviewCreate
   * @summary 编辑、批准或拒绝知识变更
   * @request POST:/api/v1/knowledge-agent/review/{action}
   * @secure
   */
  export namespace V1KnowledgeAgentReviewCreate {
    export type RequestParams = {
      /** edit/approve/reject */
      action: string;
    };
    export type RequestQuery = {};
    export type RequestBody = DomainKnowledgeReviewReq;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse & {
      data?: DomainKnowledgeChange;
    };
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentTasksList
   * @summary 获取知识任务状态
   * @request GET:/api/v1/knowledge-agent/tasks
   * @secure
   */
  export namespace V1KnowledgeAgentTasksList {
    export type RequestParams = {};
    export type RequestQuery = {
      /** 知识库 ID */
      kb_id: string;
    };
    export type RequestBody = never;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse & {
      data?: DomainKnowledgeTask[];
    };
  }

  /**
   * No description
   * @tags knowledge_agent
   * @name V1KnowledgeAgentTasksCreate
   * @summary 重试或取消知识任务
   * @request POST:/api/v1/knowledge-agent/tasks/{action}
   * @secure
   */
  export namespace V1KnowledgeAgentTasksCreate {
    export type RequestParams = {
      /** retry/cancel */
      action: string;
    };
    export type RequestQuery = {};
    export type RequestBody = DomainKnowledgeReviewReq;
    export type RequestHeaders = {};
    export type ResponseBody = DomainPWResponse;
  }
}
