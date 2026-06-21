import { useState, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Wrench, ChevronDown, ChevronRight, Loader2, AlertCircle, CheckCircle2, ShieldAlert, Check, X } from 'lucide-react';
import type { ChatMessage, ToolCallSummary } from '@/api/chat';
import { approveApproval, rejectApproval, getApproval } from '@/api/approvals';
import { cn } from '@/lib/cn';
import { useI18n } from '@/i18n/locale';

type Props = {
  message: ChatMessage;
};

export function MessageBubble({ message }: Props) {
  if (message.kind === 'tool_card' && message.tool_call) {
    return <ToolCallSummaryBlock call={fromSummary(message.tool_call)} />;
  }
  if (message.role === 'tool') return <ToolBubble message={message} />;
  if (message.role === 'user') return <UserBubble message={message} />;
  // Tool-only assistant rows (empty content + has tool_calls) shouldn't
  // appear during streaming; on history reload they would, so suppress.
  if (
    message.role === 'assistant' &&
    (!message.content || message.content.length === 0) &&
    !message.pending
  ) {
    return null;
  }
  return <AssistantBubble message={message} />;
}

// fromSummary maps the wire-level ToolCallSummary (server SSE shape) to
// the {arguments,result,...} shape the rich card already understands.
function fromSummary(tc: ToolCallSummary) {
  const args = tc.arguments ?? (tc.arguments_raw ? safeParse(tc.arguments_raw) : undefined);
  const result = tc.result ?? (tc.result_raw ? safeParse(tc.result_raw) : undefined);
  return {
    name: tc.name,
    device_id: tc.device_id,
    status: tc.status,
    duration_ms: tc.duration_ms,
    error: tc.error,
    arguments: args as Record<string, unknown> | undefined,
    result,
  };
}

function safeParse(s: string): unknown {
  try {
    return JSON.parse(s);
  } catch {
    return s;
  }
}

function UserBubble({ message }: Props) {
  // Codex-style: small, compact zinc chip pinned right. No accent color
  // — keeps the visual weight on the assistant content below.
  return (
    <div className="flex justify-end">
      <div className="max-w-[78%] rounded-2xl rounded-br-md bg-zinc-800/80 px-3.5 py-2 text-[14px] leading-relaxed text-zinc-100 ring-1 ring-zinc-700/60">
        {message.content}
      </div>
    </div>
  );
}

function AssistantBubble({ message }: Props) {
  // Codex-style: no rounded card around assistant prose. Render markdown
  // flush against the column so headings/lists/code blocks read like a
  // document. Tool calls (when attached) appear as their own rows inside
  // the same column, matching the doc-card aesthetic.
  return (
    <div className="flex flex-col items-stretch gap-2">
      {message.pending ? (
        <span className="text-zinc-500">
          <PendingDots />
        </span>
      ) : (
        <div className="md-body text-zinc-100">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>
        </div>
      )}
      {message.tool_calls?.map((tc, i) => (
        <ToolCallSummaryBlock key={`${tc.name}-${i}`} call={tc} />
      ))}
    </div>
  );
}

function ToolBubble({ message }: Props) {
  // History-reload path: the message persisted by the agent loop only
  // carries the tool name + JSON result string. We don't have args for
  // these (would need to join chat_tool_calls); show what we have.
  const result = message.content ? safeParse(message.content) : undefined;
  return (
    <ToolCallSummaryBlock
      call={{
        name: message.tool_name ?? 'tool',
        status: 'success',
        result,
      }}
    />
  );
}

function ToolCallSummaryBlock({
  call,
}: {
  call: {
    name: string;
    device_id?: number;
    status?: string;
    arguments?: Record<string, unknown> | unknown;
    result?: unknown;
    duration_ms?: number;
    error?: string;
  };
}) {
  const { tr } = useI18n();
  const [open, setOpen] = useState(false);
  const status = call.status ?? (call.error ? 'error' : 'success');
  const isPending = status === 'pending';
  const isError = status === 'error' || status === 'timeout' || !!call.error;
  const hint = argSummary(call.arguments);
  // Inline approval (HLD-017): a cloud_bash tool result that returned
  // pending_approval renders an in-conversation 批准/拒绝 card instead of a
  // plain result blob — the human confirms right here, no inbox detour.
  const approvalID = pendingApprovalID(call.result);
  if (approvalID) {
    return <PendingApprovalCard approvalID={approvalID} command={argCommandText(call.arguments)} />;
  }
  return (
    <div
      className={cn(
        'w-full overflow-hidden rounded-lg bg-zinc-900/40 text-xs ring-1',
        isError ? 'ring-red-500/30' : 'ring-zinc-800/80',
      )}
    >
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={tr(`工具调用 ${call.name}`, `Tool call ${call.name}`)}
        className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-zinc-300 hover:bg-zinc-800/40"
      >
        <StatusIcon status={status} />
        <Wrench size={12} className="text-zinc-500" />
        <span className="font-medium text-zinc-200">{call.name}</span>
        {hint && (
          <span className="truncate text-[11px] text-zinc-500" title={hint}>
            {hint}
          </span>
        )}
        {typeof call.device_id === 'number' && (
          <span className="rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] text-zinc-400">
            edge#{call.device_id}
          </span>
        )}
        <span className="ml-auto flex items-center gap-2 text-[11px] text-zinc-500">
          {typeof call.duration_ms === 'number' && call.duration_ms > 0 && (
            <span>{formatDuration(call.duration_ms)}</span>
          )}
          {isPending && <span className="text-blue-400">{tr('运行中', 'Running')}</span>}
          {isError && <span className="text-red-400">{tr('失败', 'Failed')}</span>}
          {open ? (
            <ChevronDown size={13} className="text-zinc-500" />
          ) : (
            <ChevronRight size={13} className="text-zinc-500" />
          )}
        </span>
      </button>
      {open && (
        <div className="border-t border-zinc-800/80 bg-zinc-950/40 px-3 py-2">
          {call.arguments !== undefined && (
            <div className="mb-2">
              <div className="mb-1 text-[10px] uppercase tracking-wide text-zinc-500">{tr('参数', 'Arguments')}</div>
              <pre className="max-h-48 overflow-auto text-[11px] leading-5 text-zinc-300">
                {typeof call.arguments === 'string'
                  ? call.arguments
                  : JSON.stringify(call.arguments, null, 2)}
              </pre>
            </div>
          )}
          {call.result !== undefined && (
            <div>
              <div className="mb-1 text-[10px] uppercase tracking-wide text-zinc-500">{tr('结果', 'Result')}</div>
              <pre className="max-h-72 overflow-auto text-[11px] leading-5 text-zinc-300">
                {typeof call.result === 'string'
                  ? call.result
                  : JSON.stringify(call.result, null, 2)}
              </pre>
            </div>
          )}
          {call.error && (
            <div className="mt-1 text-[11px] text-red-400">{call.error}</div>
          )}
          {!call.error && call.result === undefined && isPending && (
            <div className="text-[11px] text-zinc-500">{tr('等待结果…', 'Waiting for result…')}</div>
          )}
        </div>
      )}
    </div>
  );
}

function StatusIcon({ status }: { status?: string }) {
  if (status === 'pending') {
    return <Loader2 size={13} className="animate-spin text-blue-400" />;
  }
  if (status === 'error' || status === 'timeout') {
    return <AlertCircle size={13} className="text-red-400" />;
  }
  return <CheckCircle2 size={13} className="text-emerald-400" />;
}

// argSummary picks a compact one-line preview from the arguments object.
// Most builtin skills have a single load-bearing field (query, host,
// path, ...) — show that. Falls back to the first scalar value.
function argSummary(args: unknown): string {
  if (!args || typeof args !== 'object') return '';
  const obj = args as Record<string, unknown>;
  const preferred = ['query', 'host', 'url', 'path', 'unit', 'expr', 'instance', 'device_id'];
  for (const k of preferred) {
    const v = obj[k];
    if (typeof v === 'string' && v) return truncate(v, 80);
    if (typeof v === 'number') return String(v);
  }
  for (const [, v] of Object.entries(obj)) {
    if (typeof v === 'string' && v) return truncate(v, 80);
    if (typeof v === 'number') return String(v);
  }
  return '';
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + '…' : s;
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function PendingDots() {
  const { tr } = useI18n();
  return (
    <span className="inline-flex items-center gap-1.5 text-sm">
      <span>{tr('思考中', 'Thinking')}</span>
      <span className="inline-flex gap-0.5">
        <Dot delay={0} />
        <Dot delay={0.2} />
        <Dot delay={0.4} />
      </span>
    </span>
  );
}

function Dot({ delay }: { delay: number }) {
  return (
    <span
      className="inline-block h-1 w-1 animate-pulse-dot rounded-full bg-zinc-400"
      style={{ animationDelay: `${delay}s` }}
    />
  );
}

// --- HLD-017 inline approval -------------------------------------------

// pendingApprovalID returns the approval id when a tool result is the
// cloud_bash "pending_approval" envelope, else "".
function pendingApprovalID(result: unknown): string {
  if (result && typeof result === 'object') {
    const r = result as Record<string, unknown>;
    if (r.status === 'pending_approval' && typeof r.approval_id === 'string') return r.approval_id;
  }
  return '';
}

function argCommandText(args: unknown): string {
  if (args && typeof args === 'object') {
    const c = (args as Record<string, unknown>).command;
    if (typeof c === 'string') return c;
  }
  return '';
}

// PendingApprovalCard renders an in-conversation approve/reject prompt for a
// proposed cloud_bash command. Approve runs the command (the backend executor
// runs synchronously) and shows the result inline; reject discards it.
function PendingApprovalCard({ approvalID, command }: { approvalID: string; command: string }) {
  const { tr } = useI18n();
  const [state, setState] = useState<'loading' | 'idle' | 'busy' | 'done' | 'rejected' | 'error' | 'stale'>('loading');
  const [resultText, setResultText] = useState('');
  const [errText, setErrText] = useState('');
  const [cmd, setCmd] = useState(command);

  // Reconcile with the authoritative server status on mount. When chat
  // history is reloaded, the persisted tool message carries only the result
  // blob (no arguments, no live status), so a long-decided proposal would
  // otherwise replay with dead 批准/拒绝 buttons that 404 on click ("not
  // found"). Mirrors ztna-agent's rule: a proposal's status is read from the
  // store on replay, never trusted from the message. The approval record
  // also carries the payload, so we recover the command text here too (fixes
  // the "(命令)" placeholder on the reload path).
  useEffect(() => {
    let alive = true;
    getApproval(approvalID)
      .then((a) => {
        if (!alive) return;
        if (!cmd) {
          try {
            const p = JSON.parse(a.payload) as { command?: string };
            if (p.command) setCmd(p.command);
          } catch {
            /* payload not JSON — leave placeholder */
          }
        }
        if (a.status === 'executed') {
          setState('done');
          setResultText(a.result ?? '');
        } else if (a.status === 'rejected') {
          setState('rejected');
        } else if (a.status === 'failed') {
          setState('error');
          setErrText(a.result ?? 'failed');
        } else {
          setState('idle'); // pending — offer the buttons
        }
      })
      .catch(() => {
        // Genuinely gone (404) or unreachable: never show dead buttons —
        // point the user at the inbox instead of letting a click 404.
        if (alive) setState('stale');
      });
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [approvalID]);

  const approve = async () => {
    setState('busy');
    try {
      const a = await approveApproval(approvalID);
      if (a.status === 'failed') {
        setState('error');
        setErrText(a.result ?? 'failed');
      } else {
        setState('done');
        setResultText(a.result ?? '');
      }
    } catch (e) {
      setState('error');
      setErrText((e as Error).message);
    }
  };
  const reject = async () => {
    setState('busy');
    try {
      await rejectApproval(approvalID, '');
      setState('rejected');
    } catch (e) {
      setState('error');
      setErrText((e as Error).message);
    }
  };

  return (
    <div className="w-full overflow-hidden rounded-lg bg-amber-950/15 text-xs ring-1 ring-amber-700/40">
      <div className="flex items-center gap-2 px-3 py-2">
        <ShieldAlert size={13} className="text-amber-400" />
        <span className="font-medium text-amber-200">{tr('需要你确认才能在云端执行', 'Needs your approval to run in the cloud')}</span>
      </div>
      <div className="px-3 pb-2.5">
        <pre className="mb-2 max-h-40 overflow-auto whitespace-pre-wrap break-all rounded bg-zinc-950 p-2 text-[11px] text-zinc-300">
          {cmd || tr('(命令)', '(command)')}
        </pre>
        {state === 'loading' && (
          <div className="flex items-center gap-1.5 text-zinc-500">
            <Loader2 size={12} className="animate-spin" />
            {tr('加载审批状态…', 'Loading approval status…')}
          </div>
        )}
        {state === 'stale' && (
          <div className="text-zinc-500">
            {tr('该审批已失效或已处理，请前往「待确认」页查看。', 'This approval is gone or already handled — see the Approvals page.')}
          </div>
        )}
        {state === 'idle' && (
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => void approve()}
              className="inline-flex items-center gap-1 rounded-md border border-emerald-700 bg-emerald-950/40 px-2.5 py-1 text-emerald-300 hover:bg-emerald-900/40"
            >
              <Check size={12} />
              {tr('批准并执行', 'Approve & run')}
            </button>
            <button
              type="button"
              onClick={() => void reject()}
              className="inline-flex items-center gap-1 rounded-md border border-zinc-700 px-2.5 py-1 text-zinc-400 hover:border-red-800 hover:text-red-400"
            >
              <X size={12} />
              {tr('拒绝', 'Reject')}
            </button>
          </div>
        )}
        {state === 'busy' && <div className="flex items-center gap-1.5 text-zinc-400"><Loader2 size={12} className="animate-spin" />{tr('执行中…', 'Running…')}</div>}
        {state === 'rejected' && <div className="text-zinc-500">{tr('已拒绝，未执行', 'Rejected — not run')}</div>}
        {state === 'error' && <div className="break-all text-red-400">{errText}</div>}
        {state === 'done' && (
          <div>
            <div className="mb-1 text-emerald-400">{tr('已执行', 'Executed')}</div>
            <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-all rounded bg-zinc-950 p-2 text-[11px] text-zinc-300">{prettyResult(resultText)}</pre>
          </div>
        )}
      </div>
    </div>
  );
}

function prettyResult(s: string): string {
  if (!s) return '';
  try {
    const o = JSON.parse(s) as Record<string, unknown>;
    const parts: string[] = [];
    if (o.stdout) parts.push(String(o.stdout));
    if (o.stderr) parts.push(`[stderr] ${String(o.stderr)}`);
    if (typeof o.exit_code === 'number') parts.push(`[exit ${o.exit_code}]`);
    return parts.join('\n') || s;
  } catch {
    return s;
  }
}
