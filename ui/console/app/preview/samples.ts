import type { AgentRuntimeConfig } from '~/stores/agentRuntime';
import type { SampleAgent } from '~/stores/agents';
import type { ApprovalArg, ApprovalItem, ApprovalRequest } from '~/stores/approvals';
import type { ChannelSettings, OpenwaCheck, OpenwaFill } from '~/stores/channels';
import type { EndUserList, EndUserThread } from '~/stores/endUsers';
import type { KnowledgeSource } from '~/stores/knowledge';
import type { RunDetail, RunSummary, ThreadDetail } from '~/stores/threads';
import type { ChatScript } from '~/stores/tryIt';
import type { WebSearchSetting } from '~/stores/webSearch';

// NOTE: the only home of sample data; replace each export with the runtime and tool registry RPCs as they land.

function daysAgo(days: number): Date {
  return new Date(Date.now() - days * 86_400_000);
}

export function sampleAgents(): SampleAgent[] {
  return [
    { slug: 'yasaku-keuangan', name: 'Yasaku Keuangan', description: 'Mencatat pengeluaran dan pemasukan keluarga di yasaku', version: 4, draft: true, model: 'openai/gpt-4.1-mini', runs7d: 214, medianSeconds: 6, costCents7d: 64, updatedAt: daysAgo(0.1) },
    { slug: 'tagihan', name: 'Tagihan', description: 'Mengingatkan tagihan listrik, air dan internet', version: 2, draft: false, model: 'google/gemini-2.5-flash', runs7d: 38, medianSeconds: 4, costCents7d: 9, updatedAt: daysAgo(3) },
  ];
}

export function sampleRun(): RunDetail {
  const raw = { amount: 40000, category: 'Makan & minum', wallet: 'Cash', note: 'Kopi Kenangan', date: '2026-11-04', confirm: true };
  return {
    id: 'run_7Xp2KqLvN9sT4bWz',
    agentName: 'Yasaku Keuangan',
    version: 4,
    model: 'openai/gpt-4.1-mini',
    price: { input: 0.4, output: 1.6 },
    startedBy: 'Budi · WhatsApp DM',
    threadId: 'thr_dm8Kq2Wx',
    startedAt: new Date(Date.now() - 52_000),
    maxSteps: 12,
    approval: {
      id: 'apr_kp1',
      tool: 'yasaku__record_expense',
      title: 'Simpan Rp40.000 dari Cash?',
      args: [
        { label: 'Jumlah', value: 'Rp40.000' },
        { label: 'Kategori', value: 'Makan & minum' },
        { label: 'Dompet', value: 'Cash' },
        { label: 'Tanggal', value: '4 Nov 2026' },
        { label: 'confirm', value: 'true', mono: true, gate: true },
      ],
      raw,
      status: 'pending',
      requestedBy: 'Budi',
      requestedById: 'eu_budi1',
      expiresAt: new Date(Date.now() + 23 * 3_600_000),
    },
    events: [
      { id: 'e1', type: 'input', labelKey: 'run.event_message', name: 'Pesan', at: 0, took: 0, summary: 'beli kopi kenangan 40rb pakai cash', input: 'beli kopi kenangan 40rb pakai cash' },
      { id: 'e2', type: 'model', labelKey: 'run.event_plan', name: 'Rencana', at: 0.2, took: 1.6, costCents: 0.04, tokens: 812, summary: 'Cek dompet, lalu buat ringkasan pengeluaran.', output: 'Cek dompet, lalu buat ringkasan pengeluaran.' },
      { id: 'e3', type: 'error', name: 'yasaku__list_wallets', at: 1.9, took: 5, summary: 'yasaku did not answer in 5.0 s.', input: '{}' },
      { id: 'e4', type: 'tool', name: 'yasaku__list_wallets', at: 7.0, took: 0.6, summary: 'Retry worked. Cash Rp1.250.000, BCA Rp8.420.000, GoPay Rp310.000.', input: '{}', output: '[{ "name": "Cash", "balance": 1250000 }, { "name": "BCA", "balance": 8420000 }]' },
      { id: 'e5', type: 'model', labelKey: 'run.event_preview', name: 'Ringkasan', at: 7.7, took: 1.4, costCents: 0.05, tokens: 1204, summary: 'Preview the expense with confirm=false.' },
      { id: 'e6', type: 'tool', name: 'yasaku__record_expense', at: 9.2, took: 0.5, summary: 'Preview only (confirm=false): nothing saved.', input: JSON.stringify({ ...raw, confirm: false }), output: '{ "preview": "Simpan Rp40.000 dari Cash?" }' },
      { id: 'e7', type: 'model', labelKey: 'run.event_ask', name: 'Minta persetujuan', at: 9.8, took: 0.9, costCents: 0.03, tokens: 690, summary: 'Saving needs a person: confirm=true.' },
      { id: 'e8', type: 'approval', name: 'yasaku__record_expense', at: 10.8, took: 0, summary: 'Simpan Rp40.000 dari Cash?' },
    ],
    afterApprove: [
      { id: 'e9', type: 'tool', name: 'yasaku__record_expense', at: 62.4, took: 0.7, summary: 'Saved: tx_8811.', input: JSON.stringify(raw), output: '{ "id": "tx_8811", "saved": true }' },
      { id: 'e10', type: 'output', labelKey: 'run.event_reply', name: 'Balasan', at: 63.2, took: 1.8, costCents: 0.07, tokens: 640, summary: 'Tersimpan: Kopi Kenangan Rp40.000 dari Cash. Sisa saldo Cash Rp1.210.000.' },
    ],
    afterReject: [
      { id: 'e9', type: 'output', labelKey: 'run.event_reply', name: 'Balasan', at: 58.0, took: 0.6, costCents: 0.02, tokens: 210, summary: 'Oke, tidak saya simpan.' },
    ],
  };
}

const idMonths = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];

function idDay(d: Date): string {
  return `${d.getDate()} ${idMonths[d.getMonth()]} ${d.getFullYear()}`;
}

function minutesAgo(m: number): Date {
  return new Date(Date.now() - m * 60_000);
}

export function sampleRuns(): RunSummary[] {
  return [
    { id: 'run_4Fh8RmTq2LxW', threadId: 'thr_grp3Kd9Q', threadTitle: 'Keuangan Rumah', status: 'running', detail: { key: 'runstatus.detail_running', params: { n: '3', max: '12' } }, startedAt: minutesAgo(0.3), seconds: 6.1, tokens: 1840, costCents: 0.12 },
    { id: 'run_7Xp2KqLvN9sT4bWz', threadId: 'thr_dm8Kq2Wx', threadTitle: 'Budi', personId: 'eu_budi1', status: 'awaiting_approval', detail: { key: 'runstatus.detail_awaiting', params: { name: 'Budi' } }, startedAt: minutesAgo(2), seconds: 10.8, tokens: 2706, costCents: 0.12 },
    { id: 'run_2Bn6WqZr8YtK', threadId: 'thr_grp3Kd9Q', threadTitle: 'Keuangan Rumah', status: 'completed', startedAt: minutesAgo(45), seconds: 4.2, tokens: 1204, costCents: 0.3 },
    { id: 'run_9Lm3VcXp5HsD', threadId: 'thr_api5Tn1R', threadTitle: 'cust_0001', status: 'failed', detail: { key: 'runstatus.detail_failed' }, startedAt: minutesAgo(90), seconds: 20.4, tokens: 312, costCents: 0.02 },
    { id: 'run_5Qw1ZsKd7MnB', threadId: 'thr_try2Hx6P', threadTitle: 'Try it', status: 'cancelled', detail: { key: 'runstatus.detail_cancelled', params: { name: 'Budi' } }, startedAt: minutesAgo(130), seconds: 2.3, tokens: 410, costCents: 0.03 },
    { id: 'run_8Rt4YbNc2JvF', threadId: 'thr_grp3Kd9Q', threadTitle: 'Keuangan Rumah', status: 'interrupted', detail: { key: 'runstatus.detail_interrupted' }, startedAt: minutesAgo(300), seconds: 3.9, tokens: 980, costCents: 0.07 },
    { id: 'run_3Kp9WdLx6ZqM', threadId: 'thr_api5Tn1R', threadTitle: 'cust_0001', status: 'limit_reached', detail: { key: 'runstatus.detail_limit_cost', params: { cap: '$0.20' } }, startedAt: minutesAgo(1440), seconds: 41.0, tokens: 52_400, costCents: 20 },
    { id: 'run_6Hy2NfQs4TbW', threadId: 'thr_grp3Kd9Q', threadTitle: 'Keuangan Rumah', status: 'expired', detail: { key: 'runstatus.detail_expired' }, startedAt: minutesAgo(2880), seconds: 3.1, tokens: 720, costCents: 0.05 },
  ];
}

export function sampleWebSearch(): WebSearchSetting {
  return { provider: '', usedBy: ['Tagihan v2'] };
}

export const sampleAgentSlug = 'yasaku-keuangan';

export function sampleAgentRuntime(): AgentRuntimeConfig {
  return {
    webFetch: true,
    allowedDomains: ['bca.co.id'],
    webSearch: false,
    memory: true,
    knowledgeIds: ['kns_4mPq8XzR2vLt6WnB'],
  };
}

export const sampleInstructions = 'Kamu asisten keuangan keluarga Budi dan Sari. Catat pengeluaran dan pemasukan di yasaku. Selalu tunjukkan ringkasan dulu (confirm=false), lalu simpan setelah disetujui. Jawab singkat dalam Bahasa Indonesia.';

export function sampleKnowledgeSources(): KnowledgeSource[] {
  return [
    { id: 'kns_4mPq8XzR2vLt6WnB', name: 'Panduan kategori yasaku', kind: 'document', status: 'ready', chunks: 38, usedBy: ['Yasaku Keuangan'], updatedAt: daysAgo(6) },
    { id: 'kns_7hTr3QwY9pKx2VcN', name: 'Promo bank', kind: 'website', url: 'https://bank.example.id/promo', status: 'crawling', chunks: 0, pagesDone: 12, pagesTotal: 40, usedBy: [], updatedAt: daysAgo(0.02) },
    { id: 'kns_1bZx6NmK4sRv8LqD', name: 'Syarat KPR', kind: 'website', url: 'https://example.id/kpr', status: 'failed', chunks: 0, failure: { reason: 'robots', path: '/kpr' }, usedBy: [], updatedAt: daysAgo(2) },
  ];
}

export const samplePrompts = ['beli kopi kenangan 40rb pakai cash', 'Pengeluaran makan bulan ini berapa?', 'Transfer 500rb dari BCA ke Cash'];

function approval(id: string, tool: string, title: string, args: ApprovalArg[], raw: Record<string, unknown>): ApprovalRequest {
  return { id, tool, title, args, raw, status: 'pending' };
}

// sampleChatScript picks the reply the preview plays for a prompt: a report question, a transfer, or by default the coffee expense.
export function sampleChatScript(prompt: string): ChatScript {
  const text = prompt.toLowerCase();
  const wallets = { kind: 'tool' as const, name: 'yasaku__list_wallets', args: {}, result: [{ name: 'Cash', balance: 1250000 }, { name: 'BCA', balance: 8420000 }, { name: 'GoPay', balance: 310000 }] };
  if (text.includes('makan') || text.includes('bulan')) {
    return {
      steps: [
        { kind: 'tool', name: 'yasaku__period_report', args: { period: '2026-11', category: 'Makan & minum' }, result: { total: 1240000, count: 31 } },
        { kind: 'text', text: 'Pengeluaran Makan & minum bulan ini Rp1.240.000 dari 31 transaksi. Paling besar: Kopi Kenangan Rp40.000.' },
      ],
      afterApprove: [],
      afterReject: [],
      tokensPerStep: 640,
      centsPerStep: 0.3,
    };
  }
  if (text.includes('transfer')) {
    const raw = { from: 'BCA', to: 'Cash', amount: 500000, date: '2026-11-04', confirm: true };
    return {
      steps: [
        wallets,
        { kind: 'tool', name: 'yasaku__record_transfer', args: { ...raw, confirm: false }, result: { preview: 'Transfer Rp500.000 dari BCA ke Cash' }, preview: true },
        { kind: 'approval', approval: approval('apr_tr1', 'yasaku__record_transfer', 'Transfer Rp500.000 dari BCA ke Cash?', [
          { label: 'Jumlah', value: 'Rp500.000' },
          { label: 'Dari', value: 'BCA' },
          { label: 'Ke', value: 'Cash' },
          { label: 'Tanggal', value: '4 Nov 2026' },
          { label: 'confirm', value: 'true', mono: true, gate: true },
        ], raw) },
      ],
      afterApprove: [
        { kind: 'tool', name: 'yasaku__record_transfer', args: raw, result: { id: 'tx_8812', saved: true } },
        { kind: 'text', text: 'Sudah: transfer Rp500.000 dari BCA ke Cash. Saldo BCA Rp7.920.000, Cash Rp1.750.000.' },
      ],
      afterReject: [{ kind: 'text', text: 'Oke, transfernya tidak saya catat.' }],
      tokensPerStep: 720,
      centsPerStep: 0.4,
    };
  }
  const raw = { amount: 40000, category: 'Makan & minum', wallet: 'Cash', note: 'Kopi Kenangan', date: '2026-11-04', confirm: true };
  return {
    steps: [
      wallets,
      { kind: 'tool', name: 'yasaku__record_expense', args: { ...raw, confirm: false }, result: { preview: 'Simpan Rp40.000 dari Cash?' }, preview: true },
      { kind: 'approval', approval: approval('apr_kp1', 'yasaku__record_expense', 'Simpan Rp40.000 dari Cash?', [
        { label: 'Jumlah', value: 'Rp40.000' },
        { label: 'Kategori', value: 'Makan & minum' },
        { label: 'Dompet', value: 'Cash' },
        { label: 'Tanggal', value: '4 Nov 2026' },
        { label: 'confirm', value: 'true', mono: true, gate: true },
      ], raw) },
    ],
    afterApprove: [
      { kind: 'tool', name: 'yasaku__record_expense', args: raw, result: { id: 'tx_8811', saved: true } },
      { kind: 'text', text: 'Tersimpan: Kopi Kenangan Rp40.000 dari Cash. Sisa saldo Cash Rp1.210.000.' },
    ],
    afterReject: [{ kind: 'text', text: 'Oke, tidak saya simpan.' }],
    tokensPerStep: 812,
    centsPerStep: 0.4,
  };
}

export const sampleChannelId = 'chn_8fK2pQ';

export function sampleChannelSettings(): ChannelSettings {
  return {
    defaultOpenwaBaseUrl: 'https://openwa.altalune.id',
    defaultReplies: {
      failure: 'Sorry, something went wrong on our side. Please try again in a moment.',
      voice: 'Voice notes are not supported yet. Please send your message as text.',
      photo: 'I could not read that photo. Please describe it in text instead.',
    },
    timing: { quietMs: 3_000, quietMaxMs: 10_000, maxMessageAgeMinutes: 30 },
  };
}

export function sampleOpenwaFill(): OpenwaFill {
  return { deviceId: 'dev_yasaku-rumah', secret: 'sample-webhook-secret', deviceKey: 'sample-device-key' };
}

export function sampleWebhookUrl(channelId = sampleChannelId): string {
  return `https://oagent.altalune.id/hooks/openwa/${channelId}`;
}

// NOTE: the check answers by what was typed so every result can be clicked: device "403" refused key, "lain" wrong device, "diam" no message, secret "salah" bad signature.
export async function sampleOpenwaCheck(input: { deviceId: string, secret: string }, waitMs = 1200): Promise<OpenwaCheck> {
  await new Promise(resolve => setTimeout(resolve, waitMs));
  if (input.deviceId.includes('403')) {
    return 'key_refused';
  }
  if (input.deviceId.includes('lain')) {
    return 'device_mismatch';
  }
  if (input.deviceId.includes('diam')) {
    return 'no_message';
  }
  return input.secret === 'salah' ? 'bad_signature' : 'connected';
}

function run(id: string): RunSummary {
  return sampleRuns().find(r => r.id === id)!;
}

export function sampleThreads(): ThreadDetail[] {
  const kopi = sampleRun();
  const at = (m: number) => minutesAgo(m);
  const parkirAsked = at(2890);
  const parkirDay = idDay(new Date(parkirAsked.getTime() - 86_400_000));
  let n = 1;
  const id = () => n++;
  return [
    {
      id: 'thr_grp3Kd9Q',
      title: 'Keuangan Rumah',
      kind: 'whatsapp_group',
      people: [{ name: 'Budi', endUserId: 'eu_budi1' }, { name: 'Sari', endUserId: 'eu_sari2' }],
      lastSpeaker: 'Sari',
      lastSpeakerId: 'eu_sari2',
      lastText: 'transfer 200rb ke BCA',
      status: 'running',
      createdAt: daysAgo(12),
      updatedAt: at(0.3),
      runs: 4,
      tokens: 4744,
      costCents: 0.54,
      items: [
        { id: id(), kind: 'user', speaker: 'Budi', speakerId: 'eu_budi1', text: '@Yasaku hapus parkir 12rb kemarin', at: parkirAsked },
        { id: id(), kind: 'run', run: run('run_6Hy2NfQs4TbW') },
        { id: id(), kind: 'tool', name: 'yasaku__search_tx', args: { query: 'Parkir 12000' }, result: [{ id: 'tx_7701', amount: 12000 }], state: 'done' },
        { id: id(), kind: 'approval', approval: { id: 'apr_del1', tool: 'yasaku__delete_tx', title: 'Hapus transaksi Rp12.000 Parkir?', args: [{ label: 'Transaksi', value: `Parkir Rp12.000, ${parkirDay}` }, { label: 'id', value: 'tx_7701', mono: true }], raw: { id: 'tx_7701' }, status: 'expired', requestedBy: 'Budi', requestedById: 'eu_budi1' } },
        { id: id(), kind: 'user', speaker: 'Sari', speakerId: 'eu_sari2', text: '@Yasaku belanja pasar 215rb pakai gopay', photoPlaceholder: true, at: at(46) },
        { id: id(), kind: 'run', run: run('run_2Bn6WqZr8YtK') },
        { id: id(), kind: 'tool', name: 'yasaku__record_expense', args: { amount: 215000, category: 'Belanja pasar', wallet: 'GoPay', confirm: false }, result: { preview: 'Simpan Rp215.000 belanja pasar dari GoPay?' }, state: 'preview' },
        { id: id(), kind: 'approval', approval: { id: 'apr_ps2', tool: 'yasaku__record_expense', title: 'Simpan Rp215.000 belanja pasar dari GoPay?', args: [{ label: 'Jumlah', value: 'Rp215.000' }, { label: 'Kategori', value: 'Belanja pasar' }, { label: 'Dompet', value: 'GoPay' }, { label: 'confirm', value: 'true', mono: true, gate: true }], raw: { amount: 215000, category: 'Belanja pasar', wallet: 'GoPay', confirm: true }, status: 'approved', requestedBy: 'Sari', requestedById: 'eu_sari2', decidedBy: 'Sari', decidedById: 'eu_sari2', decidedVia: { channel: 'whatsapp', reply: 'ya' }, decidedAt: at(45) } },
        { id: id(), kind: 'tool', name: 'yasaku__record_expense', args: { amount: 215000, category: 'Belanja pasar', wallet: 'GoPay', confirm: true }, result: { id: 'tx_8790', saved: true }, state: 'done' },
        { id: id(), kind: 'agent', text: 'Tersimpan: Belanja pasar Rp215.000 dari GoPay. Sisa saldo GoPay Rp95.000.', streaming: false, at: at(45) },
        { id: id(), kind: 'user', speaker: 'Sari', speakerId: 'eu_sari2', text: '@Yasaku transfer 200rb ke BCA', at: at(0.4) },
        { id: id(), kind: 'run', run: run('run_4Fh8RmTq2LxW') },
        { id: id(), kind: 'tool', name: 'yasaku__list_wallets', args: {}, state: 'running' },
      ],
    },
    {
      id: 'thr_dm8Kq2Wx',
      title: 'Budi',
      kind: 'whatsapp_dm',
      people: [{ name: 'Budi', endUserId: 'eu_budi1' }],
      lastSpeaker: 'Budi',
      lastSpeakerId: 'eu_budi1',
      lastText: 'beli kopi kenangan 40rb pakai cash',
      status: 'awaiting_approval',
      createdAt: daysAgo(30),
      updatedAt: at(2),
      runs: 1,
      tokens: 2706,
      costCents: 0.12,
      items: [
        { id: id(), kind: 'user', speaker: 'Budi', speakerId: 'eu_budi1', text: 'beli kopi kenangan 40rb pakai cash', at: at(2) },
        { id: id(), kind: 'run', run: run('run_7Xp2KqLvN9sT4bWz') },
        { id: id(), kind: 'tool', name: 'yasaku__list_wallets', args: {}, result: [{ name: 'Cash', balance: 1250000 }], state: 'done' },
        { id: id(), kind: 'tool', name: 'yasaku__record_expense', args: { ...kopi.approval.raw, confirm: false }, result: { preview: 'Simpan Rp40.000 dari Cash?' }, state: 'preview' },
        { id: id(), kind: 'tool', name: 'yasaku__record_expense', args: kopi.approval.raw, state: 'awaiting' },
        { id: id(), kind: 'approval', approval: { ...kopi.approval } },
      ],
    },
    {
      id: 'thr_api5Tn1R',
      title: 'cust_0001',
      kind: 'api',
      people: [{ name: 'cust_0001', endUserId: 'eu_api3' }],
      lastSpeaker: 'cust_0001',
      lastSpeakerId: 'eu_api3',
      lastText: 'Pengeluaran makan bulan ini berapa?',
      status: 'failed',
      createdAt: daysAgo(3),
      updatedAt: at(90),
      runs: 2,
      tokens: 52_712,
      costCents: 20.02,
      items: [
        { id: id(), kind: 'user', speaker: 'cust_0001', speakerId: 'eu_api3', text: 'Ringkas semua transaksi tahun ini per kategori', at: at(1440) },
        { id: id(), kind: 'run', run: run('run_3Kp9WdLx6ZqM') },
        { id: id(), kind: 'tool', name: 'yasaku__period_report', args: { period: '2026' }, result: { categories: 14 }, state: 'done' },
        { id: id(), kind: 'notice', key: 'try.limit_cost', params: { cap: '$0.20' } },
        { id: id(), kind: 'user', speaker: 'cust_0001', speakerId: 'eu_api3', text: 'Pengeluaran makan bulan ini berapa?', at: at(90) },
        { id: id(), kind: 'run', run: run('run_9Lm3VcXp5HsD') },
        { id: id(), kind: 'agent', text: '', streaming: false, failed: true, at: at(90) },
      ],
    },
    {
      id: 'thr_try2Hx6P',
      title: 'Try it',
      kind: 'try',
      people: [{ name: 'Budi' }],
      lastSpeaker: 'Budi',
      lastText: 'Transfer 500rb dari BCA ke Cash',
      status: 'cancelled',
      createdAt: at(140),
      updatedAt: at(130),
      runs: 1,
      tokens: 410,
      costCents: 0.03,
      items: [
        { id: id(), kind: 'user', speaker: 'Budi', text: 'Transfer 500rb dari BCA ke Cash', at: at(130) },
        { id: id(), kind: 'run', run: run('run_5Qw1ZsKd7MnB') },
        { id: id(), kind: 'notice', key: 'try.stopped' },
      ],
    },
  ];
}

export function sampleApprovals(): ApprovalItem[] {
  const threads = sampleThreads();
  const card = (threadId: string, approvalId: string) => {
    const item = threads.find(t => t.id === threadId)!.items.find(i => i.kind === 'approval' && i.approval.id === approvalId);
    return { ...(item as { approval: ApprovalRequest }).approval };
  };
  const yasaku = { agentName: 'Yasaku Keuangan', agentSlug: 'yasaku-keuangan' };
  return [
    { ...yasaku, approval: card('thr_dm8Kq2Wx', 'apr_kp1'), threadId: 'thr_dm8Kq2Wx', channel: 'whatsapp_dm', askedAt: minutesAgo(2) },
    { ...yasaku, approval: card('thr_grp3Kd9Q', 'apr_ps2'), threadId: 'thr_grp3Kd9Q', channel: 'whatsapp_group', askedAt: minutesAgo(46) },
    { ...yasaku, approval: { id: 'apr_tr3', tool: 'yasaku__record_transfer', title: 'Transfer Rp500.000 dari BCA ke Cash?', args: [{ label: 'Jumlah', value: 'Rp500.000' }, { label: 'Dari', value: 'BCA' }, { label: 'Ke', value: 'Cash' }, { label: 'confirm', value: 'true', mono: true, gate: true }], raw: { from: 'BCA', to: 'Cash', amount: 500000, confirm: true }, status: 'rejected', requestedBy: 'Budi', decidedBy: 'Budi', decidedVia: { channel: 'console' }, decidedAt: minutesAgo(180) }, threadId: 'thr_try2Hx6P', channel: 'try', askedAt: minutesAgo(181) },
    { ...yasaku, approval: card('thr_grp3Kd9Q', 'apr_del1'), threadId: 'thr_grp3Kd9Q', channel: 'whatsapp_group', askedAt: minutesAgo(2890) },
  ];
}

export function sampleEndUsers(): EndUserList {
  const yasaku = { agentSlug: sampleAgentSlug, agentName: 'Yasaku Keuangan' };
  const group: EndUserThread = { ...yasaku, id: 'thr_grp3Kd9Q', title: 'Keuangan Rumah', kind: 'whatsapp_group', updatedAt: minutesAgo(0.3) };
  return {
    memoryOn: true,
    items: [
      {
        id: 'eu_budi1',
        name: 'Budi',
        identities: [
          { kind: 'phone', value: '+62 800 •••• 0001', linkedAt: daysAgo(30) },
          { kind: 'lid', value: '•••• 4417', linkedAt: daysAgo(30) },
        ],
        threads: [{ ...yasaku, id: 'thr_dm8Kq2Wx', title: 'Budi', kind: 'whatsapp_dm', updatedAt: minutesAgo(2) }, group],
        memories: [
          { id: 'mem_3Vx8QnT2', text: 'Budi biasanya bayar kopi pakai Cash', threadId: 'thr_dm8Kq2Wx', agentSlug: sampleAgentSlug, learnedAt: daysAgo(4) },
        ],
        lastSeenAt: minutesAgo(2),
      },
      {
        id: 'eu_sari2',
        name: 'Sari',
        identities: [
          { kind: 'phone', value: '+62 800 •••• 0002', linkedAt: daysAgo(28) },
          { kind: 'lid', value: '•••• 9023', linkedAt: daysAgo(28) },
        ],
        threads: [group],
        memories: [
          { id: 'mem_7Lp2WkR9', text: 'Sari suka mencatat belanja pasar di dompet Cash', threadId: 'thr_grp3Kd9Q', agentSlug: sampleAgentSlug, learnedAt: daysAgo(9) },
          { id: 'mem_9Dz4HbM6', text: 'Sari bayar listrik pakai BCA setiap tanggal 5', threadId: 'thr_grp3Kd9Q', agentSlug: sampleAgentSlug, learnedAt: daysAgo(2) },
        ],
        lastSeenAt: minutesAgo(0.3),
      },
      {
        id: 'eu_api3',
        name: 'cust_0001',
        identities: [{ kind: 'api', value: 'cust_0001', linkedAt: daysAgo(12) }],
        threads: [{ ...yasaku, id: 'thr_api5Tn1R', title: 'cust_0001', kind: 'api', updatedAt: minutesAgo(90) }],
        memories: [],
        lastSeenAt: minutesAgo(90),
      },
    ],
  };
}
