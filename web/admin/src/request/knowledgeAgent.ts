import httpRequest, { ContentType } from './httpClient';
import SSEClient from '@/utils/fetch';

import type {
  DomainKnowledgePolicy,
  DomainKnowledgeChange,
  DomainKnowledgeExtraction,
  DomainKnowledgeTask,
  DomainKnowledgeAudit,
  DomainConversationMessage,
} from './generated/knowledgeAgent.generated';

// The backend emits these scalar fields even though Swagger marks them optional.
export type KnowledgePolicy = Omit<Required<DomainKnowledgePolicy>, 'mode'> & {
  mode: 'off' | 'manual' | 'draft';
};
export type KnowledgeProposal = Required<DomainKnowledgeExtraction>;
export type KnowledgeChange = Omit<
  Required<DomainKnowledgeChange>,
  'proposal'
> & { proposal: KnowledgeProposal };
export type KnowledgeTask = Required<DomainKnowledgeTask>;
export type KnowledgeAudit = Required<DomainKnowledgeAudit>;
export type KnowledgeMessage = Pick<
  Required<DomainConversationMessage>,
  'id' | 'content'
> & { role: string };
export type KnowledgeEvent = {
  type: string;
  content: string;
  chunk_result?: { node_id: string; name: string };
};

export function knowledgeRequest<T>(
  path: string,
  method = 'GET',
  data: Record<string, unknown> = {},
): Promise<T> {
  return httpRequest<{ data: T }>({
    path: `/api/v1/knowledge-agent/${path}`,
    method,
    type: ContentType.Json,
    ...(method === 'GET' ? { query: data } : { body: data }),
  });
}

export const createKnowledgeChatStream = (
  onError: (error: Error) => void,
  onComplete: () => void,
) =>
  new SSEClient<KnowledgeEvent>({
    url: '/api/v1/knowledge-agent/chat',
    responseMode: 'sse-json',
    onError,
    onComplete,
  });
