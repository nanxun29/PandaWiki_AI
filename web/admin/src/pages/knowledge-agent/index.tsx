import MarkDown from '@/components/MarkDown';
import { getApiV1NodeList } from '@/request/Node';
import { getApiV1NavList } from '@/request/Nav';
import { getApiV1KnowledgeBaseUserList } from '@/request/KnowledgeBase';
import { ConstsUserKBPermission } from '@/request/types';
import {
  createKnowledgeChatStream,
  knowledgeRequest,
  KnowledgeAudit,
  KnowledgeChange,
  KnowledgeMessage,
  KnowledgePolicy,
  KnowledgeTask,
} from '@/request/knowledgeAgent';
import { useAppSelector } from '@/store';
import { message } from '@ctzhian/ui';
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Divider,
  FormControlLabel,
  MenuItem,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from '@mui/material';
import { StyledAiBubble, StyledUserBubble } from '@panda-wiki/ui/conversation';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import ProposalEditor from './ProposalEditor';

const statusNames: Record<string, string> = {
  pending: '待处理',
  running: '处理中',
  done: '处理完成',
  failed: '处理失败',
  paused: '预算暂停',
  cancelled: '已取消',
  published: '已发布',
  rejected: '已拒绝',
};

export default function KnowledgeAgent() {
  const { kb_id, kbDetail, user, modelList } = useAppSelector(
    state => state.config,
  );
  const canConfigure =
    kbDetail.perm === ConstsUserKBPermission.UserKBPermissionFullControl;
  const [tab, setTab] = useState('inbox');
  const [policy, setPolicy] = useState<KnowledgePolicy | null>(null);
  const [changes, setChanges] = useState<KnowledgeChange[]>([]);
  const [tasks, setTasks] = useState<KnowledgeTask[]>([]);
  const [audits, setAudits] = useState<KnowledgeAudit[]>([]);
  const [folders, setFolders] = useState<
    { id: string; name: string; nav_id?: string }[]
  >([]);
  const [documents, setDocuments] = useState<{ id: string; name: string }[]>(
    [],
  );
  const [navs, setNavs] = useState<{ id: string; name: string }[]>([]);
  const [members, setMembers] = useState<{ id: string; name: string }[]>([]);
  const [filter, setFilter] = useState('pending');
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<KnowledgeChange | null>(null);
  const [checked, setChecked] = useState<string[]>([]);
  const [after, setAfter] = useState('');
  const [confirmed, setConfirmed] = useState(false);
  const [source, setSource] = useState<KnowledgeMessage[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [conversation, setConversation] = useState('');
  const [messages, setMessages] = useState<KnowledgeMessage[]>([]);
  const [question, setQuestion] = useState('');
  const [streaming, setStreaming] = useState(false);
  const [toolState, setToolState] = useState('');
  const [references, setReferences] = useState<{ id: string; name: string }[]>(
    [],
  );
  const streamRef = useRef<ReturnType<typeof createKnowledgeChatStream> | null>(
    null,
  );
  const scopeRef = useRef(kb_id);

  const refresh = useCallback(async () => {
    if (!kb_id) return;
    const scope = kb_id;
    setLoading(true);
    setError('');
    try {
      const [p, c, t, a, nodes, navigation, users] = await Promise.all([
        knowledgeRequest<{ policy: KnowledgePolicy }>('policy', 'GET', {
          kb_id,
        }),
        knowledgeRequest<KnowledgeChange[]>('changes', 'GET', { kb_id }),
        knowledgeRequest<KnowledgeTask[]>('tasks', 'GET', { kb_id }),
        knowledgeRequest<KnowledgeAudit[]>('audits', 'GET', { kb_id }),
        getApiV1NodeList({ kb_id }),
        getApiV1NavList({ kb_id }),
        canConfigure
          ? getApiV1KnowledgeBaseUserList({ kb_id })
          : Promise.resolve([]),
      ]);
      if (scopeRef.current !== scope) return;
      setPolicy(p.policy);
      setChanges(c);
      setTasks(t);
      setAudits(a);
      setFolders(
        nodes
          .filter(n => n.type === 1)
          .map(n => ({ id: n.id || '', name: n.name || '', nav_id: n.nav_id })),
      );
      setDocuments(
        nodes
          .filter(n => n.type === 2)
          .map(n => ({ id: n.id || '', name: n.name || '' })),
      );
      setNavs(navigation.map(n => ({ id: n.id || '', name: n.name || '' })));
      setMembers(
        users
          .filter(
            u => u.perms !== ConstsUserKBPermission.UserKBPermissionDataOperate,
          )
          .map(u => ({ id: u.id || '', name: u.account || '' })),
      );
    } catch {
      if (scopeRef.current === scope) setError('加载失败，请刷新重试。');
    } finally {
      if (scopeRef.current === scope) setLoading(false);
    }
  }, [kb_id, canConfigure]);

  useEffect(() => {
    scopeRef.current = kb_id;
    streamRef.current?.unsubscribe();
    setConversation('');
    setMessages([]);
    setSelected(null);
    setChecked([]);
    setStreaming(false);
    void refresh();
    return () => {
      streamRef.current?.unsubscribe();
    };
  }, [kb_id, refresh]);

  useEffect(() => {
    if (!tasks.some(t => t.status === 'pending' || t.status === 'running'))
      return;
    const timer = setInterval(() => {
      void refresh();
    }, 15000);
    return () => clearInterval(timer);
  }, [tasks, refresh]);

  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setError('');
    try {
      await action();
    } catch {
      setError('操作未完成；请检查权限、版本或任务状态后重试。');
    } finally {
      setBusy(false);
    }
  };

  const review = async (
    change: KnowledgeChange,
    action: 'approve' | 'reject' | 'edit',
    content?: string,
  ) => {
    const updated = await knowledgeRequest<KnowledgeChange>(
      `review/${action}`,
      'POST',
      {
        kb_id,
        id: change.id,
        revision: change.revision,
        idempotency_key: crypto.randomUUID(),
        ...(action === 'edit' && selected
          ? {
              title: selected.proposal.title,
              nav_id: selected.proposal.nav_id,
              parent_id: selected.proposal.parent_id,
              target_document_id: selected.proposal.target_document_id,
            }
          : {}),
        ...(content === undefined ? {} : { after: content }),
      },
    );
    setSelected(updated);
    setAfter(updated.after);
    await refresh();
    message.success(
      action === 'approve'
        ? '文档已发布，索引状态请查看任务及文档学习状态'
        : action === 'reject'
          ? '已拒绝'
          : '建议已保存',
    );
  };

  const send = () => {
    if (!question.trim() || streaming) return;
    const scope = kb_id;
    const assistantID = crypto.randomUUID();
    setMessages(previous => [
      ...previous,
      { id: crypto.randomUUID(), role: 'user', content: question },
      { id: assistantID, role: 'assistant', content: '' },
    ]);
    setStreaming(true);
    setError('');
    setToolState('');
    setReferences([]);
    const client = createKnowledgeChatStream(
      () => {
        if (scopeRef.current === scope) {
          setStreaming(false);
          setError('回答失败；已生成的文字可能不完整，请重试。');
        }
      },
      () => {
        if (scopeRef.current === scope) {
          setStreaming(false);
          void refresh();
        }
      },
    );
    streamRef.current = client;
    client.subscribe(
      JSON.stringify({
        kb_id,
        conversation_id: conversation,
        message: question,
        idempotency_key: crypto.randomUUID(),
      }),
      event => {
        if (scopeRef.current !== scope) return;
        if (event.type === 'conversation_id') setConversation(event.content);
        if (event.type === 'data')
          setMessages(previous =>
            previous.map(m =>
              m.id === assistantID
                ? { ...m, content: m.content + event.content }
                : m,
            ),
          );
        if (event.type === 'knowledge_tool') {
          const [tool, state] = event.content.split(':');
          const names: Record<string, string> = {
            list_knowledge_bases: '获取知识库',
            list_folders: '获取目录',
            list_documents: '获取文档列表',
            search_knowledge: '搜索知识',
            read_document: '读取文档',
            find_similar_documents: '查找相似知识',
            propose_change: '整理知识',
          };
          setToolState(
            tool === 'propose_change' && state === 'done'
              ? '本轮回复完成后提交整理任务，内容尚未发布'
              : `${names[tool] || '知识操作'}${state === 'running' ? '中' : '完成'}`,
          );
        }
        if (event.type === 'chunk_result' && event.chunk_result) {
          const chunk = event.chunk_result;
          setReferences(previous => [
            ...previous,
            { id: chunk.node_id, name: chunk.name },
          ]);
        }
        if (event.type === 'error') {
          setError(event.content);
          setStreaming(false);
        }
      },
    );
    setQuestion('');
  };

  const originalSelected = changes.find(c => c.id === selected?.id);
  const metadataDirty =
    !!selected &&
    !!originalSelected &&
    ['title', 'nav_id', 'parent_id', 'target_document_id'].some(
      key =>
        selected.proposal[key as keyof typeof selected.proposal] !==
        originalSelected.proposal[
          key as keyof typeof originalSelected.proposal
        ],
    );
  const shown = changes.filter(
    c =>
      (!filter || c.status === filter) &&
      `${c.proposal.title} ${c.proposal.summary}`.includes(search),
  );

  return (
    <Stack gap={3} sx={{ p: { xs: 2, md: 4 }, minWidth: 0 }}>
      <Stack direction='row' justifyContent='space-between' alignItems='center'>
        <Typography variant='h5'>知识收件箱</Typography>
        <Button
          disabled={loading}
          onClick={() => {
            void refresh();
          }}
        >
          刷新
        </Button>
      </Stack>
      <Typography color='text.secondary'>
        从交流中整理知识，经审核后进入正式文档。未发布的建议不会参与知识检索。
      </Typography>
      {error && (
        <Alert severity='error' onClose={() => setError('')}>
          {error}
        </Alert>
      )}
      {policy?.mode === 'off' && (
        <Alert severity='info'>
          知识整理已关闭。原有问答保持可用，可由知识库管理员在管理设置中启用。
        </Alert>
      )}
      <Tabs
        value={tab}
        onChange={(_, value: string) => setTab(value)}
        variant='scrollable'
        allowScrollButtonsMobile
      >
        <Tab label='待审核知识' value='inbox' />
        <Tab label='知识助手' value='chat' />
        <Tab label='处理任务' value='tasks' />
        <Tab label='操作记录' value='audit' />
        {canConfigure && <Tab label='管理设置' value='settings' />}
      </Tabs>
      {loading && !policy ? (
        <CircularProgress size={24} aria-label='正在加载知识收件箱' />
      ) : null}

      {tab === 'inbox' && (
        <Stack gap={2}>
          <Stack direction={{ xs: 'column', sm: 'row' }} gap={2}>
            <TextField
              label='搜索标题与摘要'
              value={search}
              onChange={e => setSearch(e.target.value)}
              size='small'
            />
            <TextField
              label='状态'
              select
              value={filter}
              onChange={e => {
                setFilter(e.target.value);
                setSelected(null);
              }}
              size='small'
              sx={{ minWidth: 160 }}
            >
              <MenuItem value=''>全部</MenuItem>
              <MenuItem value='pending'>待审核</MenuItem>
              <MenuItem value='published'>已发布</MenuItem>
              <MenuItem value='rejected'>已拒绝</MenuItem>
            </TextField>
            <Button
              disabled={busy || !checked.length}
              onClick={() => {
                void run(async () => {
                  let approved = 0;
                  const failures: string[] = [];
                  for (const c of changes.filter(
                    item =>
                      checked.includes(item.id) &&
                      item.status === 'pending' &&
                      item.proposal.risk_level === 'low',
                  )) {
                    try {
                      await review(c, 'approve');
                      approved++;
                    } catch {
                      failures.push(c.proposal.title);
                    }
                  }
                  setChecked([]);
                  message.info(
                    `已发布 ${approved} 条${failures.length ? `；失败：${failures.join('、')}` : ''}`,
                  );
                });
              }}
            >
              批量批准低风险建议（{checked.length}）
            </Button>
          </Stack>
          {!shown.length && !loading && (
            <Alert severity='info'>
              暂无符合条件的知识。可在知识助手中讨论并整理，或启用自动草稿。
            </Alert>
          )}
          <Stack
            direction={{ xs: 'column', lg: 'row' }}
            gap={3}
            alignItems='flex-start'
          >
            <Stack
              divider={<Divider />}
              sx={{
                width: { xs: '100%', lg: selected ? 300 : '100%' },
                flexShrink: 0,
              }}
            >
              {shown.map(c => (
                <Stack key={c.id} direction='row' gap={1} sx={{ py: 2 }}>
                  {c.status === 'pending' &&
                    c.proposal.risk_level === 'low' && (
                      <Checkbox
                        aria-label={`选择${c.proposal.title}`}
                        checked={checked.includes(c.id)}
                        onChange={e =>
                          setChecked(previous =>
                            e.target.checked
                              ? [...previous, c.id]
                              : previous.filter(id => id !== c.id),
                          )
                        }
                      />
                    )}
                  <Button
                    onClick={() => {
                      setSelected(c);
                      setAfter(c.after);
                      setConfirmed(false);
                      setSource(null);
                    }}
                    sx={{ display: 'block', textAlign: 'left', flex: 1 }}
                  >
                    <Typography>{c.proposal.title}</Typography>
                    <Typography variant='body2' color='text.secondary'>
                      {c.proposal.summary}
                    </Typography>
                    <Typography variant='caption'>
                      {statusNames[c.status]} · {c.proposal.action}
                    </Typography>
                  </Button>
                </Stack>
              ))}
            </Stack>
            {selected && (
              <Stack gap={2} sx={{ minWidth: 0, flex: 1, width: '100%' }}>
                <Stack direction='row' flexWrap='wrap' gap={1}>
                  <Chip label={selected.proposal.action} />
                  <Chip
                    label={`风险：${selected.proposal.risk_level}`}
                    color={
                      selected.proposal.risk_level === 'high'
                        ? 'warning'
                        : 'default'
                    }
                  />
                </Stack>
                <Typography variant='h6'>{selected.proposal.title}</Typography>
                <Typography>{selected.proposal.reason}</Typography>
                <Typography variant='body2' color='text.secondary'>
                  目标：
                  {folders.find(f => f.id === selected.proposal.parent_id)
                    ?.name || '知识库根目录'}{' '}
                  · 基线版本 {selected.base_revision} ·{' '}
                  {selected.proposal.verification_status}
                </Typography>
                {selected.status === 'pending' && (
                  <Stack gap={2}>
                    <TextField
                      label='知识标题'
                      value={selected.proposal.title}
                      onChange={e =>
                        setSelected({
                          ...selected,
                          proposal: {
                            ...selected.proposal,
                            title: e.target.value,
                          },
                        })
                      }
                    />
                    <TextField
                      select
                      label='目标文档'
                      value={selected.proposal.target_document_id || ''}
                      onChange={e =>
                        setSelected({
                          ...selected,
                          proposal: {
                            ...selected.proposal,
                            target_document_id: e.target.value,
                          },
                        })
                      }
                      helperText='调整目标后先保存建议，再编辑正文和审核发布。'
                    >
                      <MenuItem value=''>创建新文档</MenuItem>
                      {documents.map(d => (
                        <MenuItem key={d.id} value={d.id}>
                          {d.name}
                        </MenuItem>
                      ))}
                    </TextField>
                    {!selected.proposal.target_document_id && (
                      <>
                        <TextField
                          select
                          label='导航分组'
                          value={selected.proposal.nav_id}
                          onChange={e =>
                            setSelected({
                              ...selected,
                              proposal: {
                                ...selected.proposal,
                                nav_id: e.target.value,
                                parent_id: '',
                              },
                            })
                          }
                        >
                          {navs.map(n => (
                            <MenuItem key={n.id} value={n.id}>
                              {n.name}
                            </MenuItem>
                          ))}
                        </TextField>
                        <TextField
                          select
                          label='目标目录'
                          value={selected.proposal.parent_id || ''}
                          onChange={e =>
                            setSelected({
                              ...selected,
                              proposal: {
                                ...selected.proposal,
                                parent_id: e.target.value,
                              },
                            })
                          }
                        >
                          <MenuItem value=''>根目录</MenuItem>
                          {folders
                            .filter(f => f.nav_id === selected.proposal.nav_id)
                            .map(f => (
                              <MenuItem key={f.id} value={f.id}>
                                {f.name}
                              </MenuItem>
                            ))}
                        </TextField>
                      </>
                    )}
                  </Stack>
                )}
                <ProposalEditor
                  key={`${selected.id}:${selected.revision}`}
                  before={selected.before}
                  content={after}
                  contentType={selected.content_type}
                  onChange={setAfter}
                />
                <Stack direction='row' gap={1} flexWrap='wrap'>
                  <Button
                    disabled={busy || selected.status !== 'pending'}
                    onClick={() => {
                      void run(() => review(selected, 'edit', after));
                    }}
                  >
                    保存建议
                  </Button>
                  {selected.status === 'pending' &&
                    selected.user_id === user.id && (
                      <Button
                        disabled={busy}
                        onClick={() => {
                          void run(async () => {
                            await knowledgeRequest(
                              'changes/regenerate',
                              'POST',
                              {
                                kb_id,
                                id: selected.id,
                                revision: selected.revision,
                                idempotency_key: crypto.randomUUID(),
                              },
                            );
                            setSelected(null);
                            await refresh();
                            message.success('已提交重新提取任务');
                          });
                        }}
                      >
                        重新提取
                      </Button>
                    )}
                  {selected.status === 'published' &&
                    selected.proposal.action !== 'CREATE' && (
                      <Button
                        disabled={busy}
                        onClick={() => {
                          void run(async () => {
                            const inverse =
                              await knowledgeRequest<KnowledgeChange>(
                                'changes/rollback',
                                'POST',
                                {
                                  kb_id,
                                  id: selected.id,
                                  revision: selected.revision,
                                  idempotency_key: crypto.randomUUID(),
                                },
                              );
                            setSelected(inverse);
                            setAfter(inverse.after);
                            setFilter('pending');
                            setConfirmed(false);
                            await refresh();
                            message.success('回滚建议已创建，请审核发布');
                          });
                        }}
                      >
                        创建回滚建议
                      </Button>
                    )}
                  <Button
                    disabled={busy}
                    onClick={() => {
                      void run(async () => {
                        setSource(
                          await knowledgeRequest<KnowledgeMessage[]>(
                            'messages',
                            'GET',
                            {
                              kb_id,
                              conversation_id: selected.conversation_id,
                            },
                          ),
                        );
                      });
                    }}
                  >
                    查看来源聊天（仅所有者）
                  </Button>
                  {selected.document_id && (
                    <Button
                      component={Link}
                      to={`/doc/editor/${selected.document_id}`}
                    >
                      打开目标文档
                    </Button>
                  )}
                </Stack>
                {source && (
                  <Box sx={{ maxHeight: 320, overflow: 'auto' }}>
                    {source
                      .filter(m =>
                        selected.proposal.source_message_ids.includes(m.id),
                      )
                      .map(m => (
                        <Box key={m.id} sx={{ mb: 2 }}>
                          <Typography variant='caption'>
                            {m.role === 'user' ? '用户' : 'AI'}
                          </Typography>
                          <MarkDown content={m.content} />
                        </Box>
                      ))}
                  </Box>
                )}
                {selected.status === 'pending' && (
                  <>
                    {metadataDirty && (
                      <Alert severity='info'>
                        目标或标题已调整，请先保存建议，再核对正文并发布。
                      </Alert>
                    )}
                    {selected.proposal.risk_level !== 'low' && (
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={confirmed}
                            onChange={e => setConfirmed(e.target.checked)}
                          />
                        }
                        label='我已核对来源、修改内容和风险，确认发布此变更'
                      />
                    )}
                    <Stack direction='row' gap={2}>
                      <Button
                        variant='contained'
                        disabled={
                          busy ||
                          metadataDirty ||
                          (selected.proposal.risk_level !== 'low' && !confirmed)
                        }
                        onClick={() => {
                          void run(() => review(selected, 'approve', after));
                        }}
                      >
                        批准并发布
                      </Button>
                      <Button
                        disabled={busy}
                        color='error'
                        onClick={() => {
                          void run(() => review(selected, 'reject'));
                        }}
                      >
                        拒绝
                      </Button>
                    </Stack>
                  </>
                )}
              </Stack>
            )}
          </Stack>
        </Stack>
      )}

      {tab === 'chat' && (
        <Stack gap={2} sx={{ maxWidth: 900 }}>
          <Typography variant='body2' color='text.secondary'>
            此会话仅向当前后台用户开放。可以询问知识，或要求把讨论整理为待审核知识。
          </Typography>
          <Stack direction='row' gap={1}>
            <Button
              disabled={streaming}
              onClick={() => {
                setConversation('');
                setMessages([]);
                setReferences([]);
                setToolState('');
              }}
            >
              新会话
            </Button>
            <Button
              disabled={
                busy || streaming || !conversation || policy?.mode === 'off'
              }
              onClick={() => {
                void run(async () => {
                  const task = await knowledgeRequest<KnowledgeTask | null>(
                    'extract',
                    'POST',
                    {
                      kb_id,
                      conversation_id: conversation,
                      idempotency_key: crypto.randomUUID(),
                    },
                  );
                  if (task)
                    message.success('整理任务已提交，完成后请到收件箱审核');
                  else message.info('暂无新增的完整对话可整理');
                  await refresh();
                });
              }}
            >
              整理当前对话
            </Button>
          </Stack>
          <Stack gap={3} aria-live='polite' sx={{ minHeight: 180 }}>
            {messages.map(m =>
              m.role === 'user' ? (
                <StyledUserBubble key={m.id}>{m.content}</StyledUserBubble>
              ) : (
                <StyledAiBubble key={m.id}>
                  <MarkDown
                    content={
                      m.content || (streaming ? '正在回答…' : '未生成回答')
                    }
                  />
                </StyledAiBubble>
              ),
            )}
          </Stack>
          {!!references.length && (
            <Stack direction='row' gap={1} flexWrap='wrap'>
              {references.map((r, i) => (
                <Button
                  key={`${r.id}:${i}`}
                  component={Link}
                  to={`/doc/editor/${r.id}`}
                >
                  {r.name}
                </Button>
              ))}
            </Stack>
          )}
          {toolState && <Alert severity='info'>知识操作：{toolState}</Alert>}
          {changes
            .filter(c => c.conversation_id === conversation)
            .map(c => (
              <Alert
                key={c.id}
                severity={c.status === 'published' ? 'success' : 'info'}
                action={
                  <Button
                    onClick={() => {
                      setTab('inbox');
                      setFilter(c.status);
                      setSelected(c);
                      setAfter(c.after);
                      setConfirmed(false);
                    }}
                  >
                    查看
                  </Button>
                }
              >
                {c.proposal.title} · {statusNames[c.status]}
              </Alert>
            ))}
          <TextField
            label='向知识助手提问'
            multiline
            minRows={2}
            value={question}
            disabled={streaming}
            onChange={e => setQuestion(e.target.value)}
          />
          <Stack direction='row' gap={2}>
            <Button
              variant='contained'
              disabled={streaming || !question.trim()}
              onClick={send}
            >
              发送
            </Button>
            {streaming && (
              <Button
                onClick={() => {
                  streamRef.current?.unsubscribe();
                  setStreaming(false);
                  setToolState('回答已停止');
                }}
              >
                停止回答
              </Button>
            )}
          </Stack>
        </Stack>
      )}

      {tab === 'tasks' && (
        <Stack divider={<Divider />}>
          {!tasks.length && (
            <Typography color='text.secondary'>暂无知识处理任务。</Typography>
          )}
          {tasks.map(t => (
            <Stack
              key={t.id}
              direction={{ xs: 'column', sm: 'row' }}
              gap={2}
              sx={{ py: 2 }}
              alignItems='flex-start'
            >
              <Box sx={{ flex: 1 }}>
                <Typography>
                  {t.kind === 'index' ? '发布索引投递' : '聊天知识整理'} ·{' '}
                  {statusNames[t.status]}
                </Typography>
                <Typography variant='body2' color='text.secondary'>
                  {t.error ||
                    (t.kind === 'index' && t.status === 'done'
                      ? '已投递原索引队列，学习结果以文档状态为准'
                      : `尝试 ${t.attempts} 次`)}
                </Typography>
              </Box>
              {['failed', 'paused'].includes(t.status) && (
                <Button
                  disabled={busy}
                  onClick={() => {
                    void run(async () => {
                      await knowledgeRequest('tasks/retry', 'POST', {
                        kb_id,
                        id: t.id,
                      });
                      await refresh();
                    });
                  }}
                >
                  重试
                </Button>
              )}
              {t.kind === 'extract' &&
                ['pending', 'running', 'paused'].includes(t.status) && (
                  <Button
                    disabled={busy}
                    onClick={() => {
                      void run(async () => {
                        await knowledgeRequest('tasks/cancel', 'POST', {
                          kb_id,
                          id: t.id,
                        });
                        await refresh();
                      });
                    }}
                  >
                    取消
                  </Button>
                )}
            </Stack>
          ))}
        </Stack>
      )}

      {tab === 'audit' && (
        <Stack divider={<Divider />}>
          {!audits.length && (
            <Typography color='text.secondary'>暂无操作记录。</Typography>
          )}
          {audits.map(a => (
            <Stack
              key={a.id}
              direction={{ xs: 'column', sm: 'row' }}
              gap={2}
              sx={{ py: 2 }}
            >
              <Typography>
                {a.action} · {a.result}
              </Typography>
              <Typography variant='body2' color='text.secondary'>
                {new Date(a.created_at).toLocaleString('zh-CN')}
              </Typography>
            </Stack>
          ))}
        </Stack>
      )}

      {tab === 'settings' && canConfigure && policy && (
        <Stack gap={3} sx={{ maxWidth: 680 }}>
          <TextField
            label='自动沉淀模式'
            select
            value={policy.mode}
            onChange={e =>
              setPolicy({
                ...policy,
                mode: e.target.value as KnowledgePolicy['mode'],
              })
            }
          >
            <MenuItem value='off'>关闭</MenuItem>
            <MenuItem value='manual'>手动保存</MenuItem>
            <MenuItem value='draft'>自动草稿，人工审核发布</MenuItem>
          </TextField>
          <TextField
            label='知识整理模型'
            select
            value={policy.model_id}
            onChange={e => setPolicy({ ...policy, model_id: e.target.value })}
          >
            <MenuItem value=''>复用已配置的聊天模型</MenuItem>
            {modelList
              ?.filter(m => m.type === 'chat')
              .map(m => (
                <MenuItem key={m.id} value={m.id}>
                  {m.model}
                </MenuItem>
              ))}
          </TextField>
          <FormControlLabel
            control={
              <Checkbox
                checked={policy.allow_model_transfer}
                onChange={e =>
                  setPolicy({
                    ...policy,
                    allow_model_transfer: e.target.checked,
                  })
                }
              />
            }
            label='允许将授权聊天与相关文档发送给上述已配置模型服务进行整理'
          />
          <TextField
            label='授权用户'
            select
            value={policy.user_ids}
            SelectProps={{ multiple: true }}
            onChange={e => {
              const value = e.target.value;
              setPolicy({
                ...policy,
                user_ids: typeof value === 'string' ? value.split(',') : value,
              });
            }}
          >
            {members.map(m => (
              <MenuItem key={m.id} value={m.id}>
                {m.name}
              </MenuItem>
            ))}
          </TextField>
          <FormControlLabel
            control={
              <Checkbox
                checked={policy.user_ids.includes(user.id || '')}
                onChange={e =>
                  setPolicy({
                    ...policy,
                    user_ids: e.target.checked
                      ? [...new Set([...policy.user_ids, user.id || ''])]
                      : policy.user_ids.filter(id => id !== user.id),
                  })
                }
              />
            }
            label='授权当前用户使用知识管理工具与自动提取'
          />
          <TextField
            label='授权目录'
            select
            value={policy.folder_ids}
            SelectProps={{ multiple: true }}
            onChange={e => {
              const value = e.target.value;
              setPolicy({
                ...policy,
                folder_ids:
                  typeof value === 'string' ? value.split(',') : value,
              });
            }}
            helperText='选择全部目录代表授权本知识库内的所有目录；不选择则不能写入。'
          >
            <MenuItem value='*'>全部目录</MenuItem>
            <MenuItem value=''>根目录</MenuItem>
            {folders.map(f => (
              <MenuItem key={f.id} value={f.id}>
                {f.name}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            label='每日模型调用次数上限'
            type='number'
            value={policy.daily_calls}
            onChange={e =>
              setPolicy({ ...policy, daily_calls: Number(e.target.value) })
            }
            inputProps={{ min: 1, max: 10000 }}
          />
          <Alert severity='info'>
            自动草稿在累计三轮完整对话或闲置五分钟后整理。发布需要人工审核，预算耗尽后任务暂停。
          </Alert>
          <Button
            variant='contained'
            disabled={busy}
            onClick={() => {
              void run(async () => {
                await knowledgeRequest('policy', 'PUT', policy);
                message.success('知识管理设置已保存');
                await refresh();
              });
            }}
          >
            保存设置
          </Button>
        </Stack>
      )}
    </Stack>
  );
}
