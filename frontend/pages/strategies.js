import { useCallback, useEffect, useMemo, useState } from 'react';

import { requestJson } from '../lib/api';
import { useAuth } from '../lib/auth-context';
import { isAuthRequiredError } from '../lib/auth-storage';
import Head from 'next/head';
import {
  getInputAttributes,
  validateStrategyParams,
} from '../lib/strategy-form';
import {
  STRATEGY_PRESETS,
  buildDraftFromStrategy,
  buildPayloadFromDraft,
  buildPresetDefinition,
  createDraftFromType,
  getStrategyPresetByImplementation,
  getStrategyPresetByType,
  resolveStrategyDescription,
} from '../lib/strategy-presets';

const STATUS_OPTIONS = [
  { value: 'draft', label: '草稿' },
  { value: 'active', label: '启用' },
  { value: 'archived', label: '归档' },
];

export default function StrategyLibraryPage() {
  const { openAuthModal, isLoggedIn, ready, user } = useAuth();
  const [strategies, setStrategies] = useState([]);
  const [selectedId, setSelectedId] = useState('');
  const [selectedDetail, setSelectedDetail] = useState(null);
  const [mode, setMode] = useState('view');
  const [draft, setDraft] = useState(null);
  const [draftOrigin, setDraftOrigin] = useState(null);
  const [pendingAction, setPendingAction] = useState(null);
  const [createMenuOpen, setCreateMenuOpen] = useState(false);
  const [loadingList, setLoadingList] = useState(true);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [error, setError] = useState('');
  const [errorNeedsLogin, setErrorNeedsLogin] = useState(false);
  const [success, setSuccess] = useState('');

  // ── AI 生成策略弹窗 ──
  const [aiDialogOpen, setAiDialogOpen] = useState(false);
  const [aiTicker, setAiTicker] = useState('');
  const [aiLoading, setAiLoading] = useState(false);
  const [aiResult, setAiResult] = useState(null);
  const [aiBacktestLoading, setAiBacktestLoading] = useState(false);
  const [aiBacktestData, setAiBacktestData] = useState(null);
  const [aiError, setAiError] = useState('');
  const authIdentityKey = String(user?.id || user?.email || '');

  const updateError = (nextError, nextNeedsLogin = false) => {
    setError(nextError);
    setErrorNeedsLogin(nextNeedsLogin);
  };

  const applyRequestError = (err, fallbackText) => {
    updateError(err.message || fallbackText, isAuthRequiredError(err));
  };

  const activePreset = useMemo(() => {
    if (mode === 'create' || mode === 'edit') {
      return getStrategyPresetByType(draft?.typeKey);
    }
    return getStrategyPresetByImplementation(selectedDetail?.implementation_key);
  }, [draft?.typeKey, mode, selectedDetail?.implementation_key]);

  const isDirty = useMemo(
    () => mode !== 'view' && !areDraftsEqual(draft, draftOrigin),
    [draft, draftOrigin, mode],
  );

  const workspaceTitle = mode === 'create'
    ? draft?.name || '新建策略'
    : selectedDetail?.name || '策略工作区';

  const workspaceDescription = mode === 'create'
    ? '当前正在新建策略。系统已自动生成默认名称，你只需要继续完善状态、说明和参数。'
    : mode === 'edit'
      ? '当前正在编辑这条策略。保存成功后将自动回到只读状态。'
      : '';

  const detailDescription = resolveStrategyDescription(selectedDetail?.description, activePreset);
  const canDeleteSelected = mode === 'view'
    && Boolean(selectedDetail?.id)
    && Boolean(String(selectedDetail?.user_id || '').trim())
    && !loadingDetail;

  useEffect(() => {
    if (!ready) return;
    loadStrategies();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, isLoggedIn, authIdentityKey]);

  const fetchStrategyCollection = async () => {
    const data = await requestJson('/api/strategies', undefined, '加载策略列表失败');
    const items = data?.items || [];
    setStrategies(items);
    return items;
  };

  const loadStrategies = async (preferredId) => {
    setLoadingList(true);
    updateError('');
    try {
      const items = await fetchStrategyCollection();
      const nextId = preferredId || selectedId || items[0]?.id || '';
      if (nextId) {
        await loadStrategyDetail(nextId);
      } else {
        resetWorkspace();
      }
    } catch (err) {
      applyRequestError(err, '加载策略列表失败');
    } finally {
      setLoadingList(false);
    }
  };

  const loadStrategyDetail = async (strategyId) => {
    if (!strategyId) return;
    setLoadingDetail(true);
    updateError('');
    try {
      const data = await requestJson(`/api/strategies/${strategyId}`, undefined, '加载策略详情失败');
      const nextDetail = data?.item || null;
      const nextDraft = nextDetail ? buildDraftFromStrategy(nextDetail) : null;
      setSelectedId(strategyId);
      setSelectedDetail(nextDetail);
      setMode('view');
      setDraft(nextDraft);
      setDraftOrigin(nextDraft);
      setCreateMenuOpen(false);
    } catch (err) {
      const message = String(err?.message || '');
      if (message.includes('strategy not found')) {
        try {
          const items = await fetchStrategyCollection();
          const fallbackId = items[0]?.id || '';
          if (!items.some((item) => item.id === strategyId)) {
            if (fallbackId && fallbackId !== strategyId) {
              await loadStrategyDetail(fallbackId);
            } else if (!fallbackId) {
              resetWorkspace();
            }
            updateError('该策略当前不可访问（可能已被删除，或登录态已过期）。如果这是你新建的策略，请重新登录后重试。', true);
            return;
          }
        } catch {
          // 忽略刷新失败，继续走原始错误展示
        }
      }
      applyRequestError(err, '加载策略详情失败');
    } finally {
      setLoadingDetail(false);
    }
  };

  const resetWorkspace = () => {
    setSelectedId('');
    setSelectedDetail(null);
    setMode('view');
    setDraft(null);
    setDraftOrigin(null);
    setCreateMenuOpen(false);
    setDeleteConfirmOpen(false);
  };

  const requestWorkspaceAction = async (action) => {
    updateError('');
    setSuccess('');
    setCreateMenuOpen(false);
    if (shouldConfirmBeforeAction(action, { isDirty, mode, selectedId })) {
      setPendingAction(action);
      return;
    }
    await executeWorkspaceAction(action);
  };

  const executeWorkspaceAction = async (action) => {
    if (!action) return;

    if (action.type === 'create') {
      enterCreateMode(action.strategyType);
      return;
    }

    if (action.type === 'select') {
      if (action.strategyId === selectedId && mode === 'view') {
        return;
      }
      await loadStrategyDetail(action.strategyId);
    }
  };

  const enterCreateMode = (strategyType) => {
    const nextDraft = createDraftFromType(strategyType, strategies);
    setMode('create');
    setDraft(nextDraft);
    setDraftOrigin(nextDraft);
  };

  const startEdit = () => {
    if (!selectedDetail) return;
    const nextDraft = buildDraftFromStrategy(selectedDetail);
    setSuccess('');
    updateError('');
    setMode('edit');
    setDraft(nextDraft);
    setDraftOrigin(nextDraft);
  };

  const cancelEditing = () => {
    setPendingAction(null);
    updateError('');
    setSuccess('');
    setCreateMenuOpen(false);
    if (selectedDetail) {
      const nextDraft = buildDraftFromStrategy(selectedDetail);
      setMode('view');
      setDraft(nextDraft);
      setDraftOrigin(nextDraft);
      return;
    }
    resetWorkspace();
  };

  const requestDeleteStrategy = () => {
    if (!canDeleteSelected) return;
    setDeleteConfirmOpen(true);
    updateError('');
    setSuccess('');
  };

  const handleDeleteStrategy = async () => {
    if (!selectedDetail?.id || deleting) return;

    setDeleting(true);
    updateError('');
    setSuccess('');
    try {
      await requestJson(
        `/api/strategies/${selectedDetail.id}`,
        { method: 'DELETE' },
        '删除策略失败',
      );
      const deletedID = selectedDetail.id;
      const items = await fetchStrategyCollection();
      const fallbackId = items.find((item) => item.id !== deletedID)?.id || items[0]?.id || '';
      setDeleteConfirmOpen(false);
      if (fallbackId) {
        await loadStrategyDetail(fallbackId);
      } else {
        resetWorkspace();
      }
      setSuccess('策略已删除。');
    } catch (err) {
      applyRequestError(err, '删除策略失败');
    } finally {
      setDeleting(false);
    }
  };

  const updateDraftField = (key, value) => {
    setDraft((prev) => ({ ...prev, [key]: value }));
  };

  const updateParamField = (key, value) => {
    setDraft((prev) => ({
      ...prev,
      params: {
        ...(prev?.params || {}),
        [key]: value,
      },
    }));
  };

  const handleSave = async (afterAction = null) => {
    if (!draft) return false;

    const preset = getStrategyPresetByType(draft.typeKey);
    const definition = buildPresetDefinition(preset);
    const validationError = validateDraft(draft, definition, preset);
    if (validationError) {
      updateError(validationError);
      return false;
    }

    setSaving(true);
    updateError('');
    setSuccess('');
    try {
      const payload = buildPayloadFromDraft(draft);
      const isCreating = mode === 'create';
      const data = await requestJson(
        isCreating ? '/api/strategies' : `/api/strategies/${selectedId}`,
        {
          method: isCreating ? 'POST' : 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        },
        '保存策略失败',
      );

      const saved = data?.item || null;
      if (saved?.id) {
        setSelectedId(saved.id);
        setSelectedDetail(saved);
      }

      await fetchStrategyCollection();

      if (afterAction) {
        await executeWorkspaceAction(afterAction);
      } else if (saved?.id) {
        await loadStrategyDetail(saved.id);
      } else {
        setMode('view');
      }

      setSuccess(isCreating ? '策略已创建。' : '策略已更新。');
      return true;
    } catch (err) {
      applyRequestError(err, '保存策略失败');
      return false;
    } finally {
      setSaving(false);
    }
  };

  const handleSaveAndContinue = async () => {
    const action = pendingAction;
    setPendingAction(null);
    await handleSave(action);
  };

  const handleDiscardAndContinue = async () => {
    const action = pendingAction;
    setPendingAction(null);
    await executeWorkspaceAction(action);
  };

  // ── AI 生成策略 ──

  const openAIDialog = useCallback(() => {
    if (!isLoggedIn) {
      openAuthModal('login', '登录后可使用 AI 智能生成策略。');
      return;
    }
    setAiTicker('');
    setAiResult(null);
    setAiBacktestData(null);
    setAiError('');
    setAiLoading(false);
    setAiBacktestLoading(false);
    setAiDialogOpen(true);
  }, [isLoggedIn, openAuthModal]);

  const handleAIGenerate = async () => {
    const trimmed = aiTicker.trim();
    if (!trimmed) { setAiError('请输入股票代码'); return; }
    setAiLoading(true);
    setAiError('');
    setAiResult(null);
    setAiBacktestData(null);
    setAiBacktestLoading(false);
    try {
      // 第一阶段：AI 推荐（快速返回）
      const data = await requestJson('/api/strategies/ai-generate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ticker: trimmed }),
      });
      setAiResult(data || null);
      setAiLoading(false);

      // 第二阶段：自动回测验证 + 迭代优化（异步）
      const rec = data?.recommendation;
      if (rec?.implementation_key && rec?.params && rec?.market_summary?.ticker) {
        setAiBacktestLoading(true);
        try {
          const btData = await requestJson('/api/strategies/ai-generate/backtest', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              symbol: rec.market_summary.ticker,
              implementation_key: rec.implementation_key,
              params: rec.params,
            }),
          });
          setAiBacktestData(btData || null);
          // 如果迭代调参后有更优参数，更新推荐结果
          if (btData?.best_params && Object.keys(btData.best_params).length > 0) {
            setAiResult((prev) => prev ? {
              ...prev,
              recommendation: { ...prev.recommendation, params: btData.best_params },
            } : prev);
          }
        } catch (btErr) {
          // 回测失败不影响推荐结果展示
          console.warn('AI backtest failed:', btErr);
        } finally {
          setAiBacktestLoading(false);
        }
      }
    } catch (err) {
      setAiError(err.message || 'AI 生成策略失败');
      setAiLoading(false);
    }
  };

  const handleAIAdopt = () => {
    const rec = aiResult?.recommendation;
    if (!rec) return;
    const preset = getStrategyPresetByImplementation(rec.implementation_key);
    if (!preset) {
      setAiError('AI 推荐的策略类型不可用，请重试');
      return;
    }
    const nextDraft = createDraftFromType(preset.typeKey, strategies);
    // 用 AI 推荐的策略名称和参数覆盖默认值
    nextDraft.name = rec.strategy_label || nextDraft.name;
    nextDraft.description = rec.reason || preset.defaultDescription;
    if (rec.params) {
      nextDraft.params = { ...nextDraft.params, ...rec.params };
    }
    setMode('create');
    setDraft(nextDraft);
    setDraftOrigin(nextDraft);
    setAiDialogOpen(false);
    setSuccess('AI 推荐已填入，请检查参数后点击「创建策略」。');
  };

  const showWorkspaceEmpty = mode !== 'create' && !selectedDetail && !loadingDetail;

  return (
    <div className="max-w-7xl mx-auto space-y-6 pb-12">
      <Head>
        <title>策略库 — 卧龙AI量化交易台</title>
        <meta name="description" content="卧龙AI量化交易台策略库 — 创建和管理量化策略，支持趋势跟踪、均线交叉、RSI 等多种策略类型，AI 智能生成策略并自动回测验证。" />
        <link rel="canonical" href="https://wolongtrader.top/strategies" />
      </Head>
      <section className="bg-card border border-border rounded-2xl p-6 md:p-8">
        <div className="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
          <div className="space-y-3 max-w-3xl">
            <span className="inline-flex items-center rounded-full border border-primary/30 bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
              Wolong Pro · 策略库
            </span>
            <div>
              <h1 className="text-3xl md:text-4xl font-semibold tracking-tight">策略库</h1>
              <p className="mt-3 text-sm md:text-base text-white/65 leading-7">
                你可以在这里维护多条策略，并持续调整名称、状态、说明与参数。
              </p>
            </div>
          </div>
          <div className="grid grid-cols-1 gap-3 text-sm text-white/70 md:min-w-[180px]">
            <MiniStat label="策略数量" value={`${strategies.length} 条`} />
          </div>
        </div>
      </section>

      <section className="grid gap-6 xl:grid-cols-[0.92fr_1.08fr]">
        <PanelCard
          title="策略列表"
          description="你可以在这里维护多条策略。"
          action={
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={openAIDialog}
                className="inline-flex items-center gap-1.5 rounded-xl bg-gradient-to-r from-indigo-500 to-violet-500 px-4 py-2 text-xs font-semibold text-white shadow-[0_0_16px_rgba(99,102,241,0.35)] transition-all duration-300 hover:scale-[1.03] hover:shadow-[0_0_24px_rgba(99,102,241,0.5)] active:scale-[0.98] animate-ai-glow"
              >
                ✨ AI 生成
              </button>
              <CreateStrategyDropdown
                open={createMenuOpen}
                onToggle={() => setCreateMenuOpen((prev) => !prev)}
                onSelect={(strategyType) => requestWorkspaceAction({ type: 'create', strategyType })}
              />
            </div>
          }
        >
          <div className="space-y-3">
            {loadingList ? (
              <EmptyState text="正在加载策略列表..." />
            ) : strategies.length === 0 ? (
              <EmptyState text="当前还没有策略，请先通过“新建策略”开始创建。" />
            ) : (
              strategies.map((strategy) => {
                const preset = getStrategyPresetByImplementation(strategy.implementation_key);
                const isSelected = selectedId === strategy.id;
                return (
                  <button
                    key={strategy.id}
                    type="button"
                    onClick={() => requestWorkspaceAction({ type: 'select', strategyId: strategy.id })}
                    className={`w-full rounded-2xl border p-4 text-left transition ${
                      isSelected ? 'border-primary bg-primary/10' : 'border-border bg-black/20 hover:border-white/20'
                    }`}
                  >
                    <div className="flex items-center justify-between gap-3">
                      <div className="text-sm font-medium text-white">{strategy.name}</div>
                      <StatusBadge status={strategy.status} />
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-white/45">
                      <span className="rounded-full border border-white/10 bg-white/5 px-2 py-1">{preset?.shortLabel || preset?.typeLabel || '策略'}</span>
                    </div>
                    <div className="mt-3 text-sm leading-6 text-white/60">{strategy.description_summary || '暂无说明'}</div>
                    <div className="mt-3 text-xs text-white/35">更新于 {formatDateTime(strategy.updated_at)}</div>
                  </button>
                );
              })
            )}
          </div>
        </PanelCard>

        <section className="rounded-2xl border border-border bg-card p-6">
          <div className="flex flex-col gap-5 border-b border-white/5 pb-5">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div className="space-y-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="inline-flex rounded-full border border-white/10 bg-black/20 px-2.5 py-1 text-xs font-medium text-white/65">
                    {mode === 'create' ? '新建中' : mode === 'edit' ? '编辑中' : '只读详情'}
                  </span>
                  {selectedDetail?.status && mode !== 'create' ? <StatusBadge status={selectedDetail.status} /> : null}
                  {isDirty ? (
                    <span className="inline-flex rounded-full border border-amber-400/30 bg-amber-400/10 px-2.5 py-1 text-xs font-medium text-amber-200">
                      当前有未保存修改
                    </span>
                  ) : null}
                </div>
                <div>
                  <h2 className="text-2xl font-semibold text-white">{workspaceTitle}</h2>
                  {workspaceDescription ? <p className="mt-2 max-w-3xl text-sm leading-6 text-white/55">{workspaceDescription}</p> : null}
                </div>
                {selectedDetail && mode !== 'create' ? (
                  <div className="flex flex-wrap gap-3 text-xs text-white/40">
                    <span>版本：v{selectedDetail.version}</span>
                    <span>更新时间：{formatDateTime(selectedDetail.updated_at)}</span>
                  </div>
                ) : mode === 'create' ? (
                  <div className="text-xs text-white/40">
                    系统已自动生成默认名称，你可以直接修改名称并完善参数。
                  </div>
                ) : null}
              </div>

              <div className="flex flex-wrap items-center gap-2">
                {mode === 'view' ? (
                  <>
                    <button
                      type="button"
                      onClick={startEdit}
                      disabled={!selectedDetail || loadingDetail}
                      className="rounded-xl bg-primary px-4 py-2 text-xs font-semibold text-white whitespace-nowrap transition hover:bg-orange-500 disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      编辑当前策略
                    </button>
                    {canDeleteSelected ? (
                      <button
                        type="button"
                        onClick={requestDeleteStrategy}
                        className="rounded-xl border border-negative/40 bg-negative/10 px-3 py-2 text-xs font-semibold text-red-200 transition hover:bg-negative/20"
                      >
                        删除策略
                      </button>
                    ) : null}
                  </>
                ) : (
                  <>
                    <button
                      type="button"
                      onClick={cancelEditing}
                      className="rounded-xl border border-white/10 bg-black/20 px-3 py-2 text-xs font-medium text-white/75 transition hover:border-white/20 hover:text-white"
                    >
                      {mode === 'create' ? '取消创建' : '取消编辑'}
                    </button>
                    <button
                      type="button"
                      onClick={() => handleSave()}
                      disabled={saving}
                      className="rounded-xl bg-primary px-3 py-2 text-xs font-semibold text-white transition hover:bg-orange-500 disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      {saving ? '保存中...' : mode === 'create' ? '创建策略' : '保存修改'}
                    </button>
                  </>
                )}
              </div>
            </div>
          </div>

          <div className="mt-6 space-y-6">
            {loadingDetail ? (
              <EmptyState text="正在加载策略详情..." />
            ) : showWorkspaceEmpty ? (
              <EmptyState text="请选择左侧策略，或通过“新建策略”开始创建。" />
            ) : (
              <>
                <SectionBlock title="基础信息">
                  {mode === 'view' ? (
                    <div className="space-y-5">
                      <DetailGrid>
                        <DetailItem label="策略名称" value={selectedDetail?.name} />
                        <DetailItem label="状态" value={selectedDetail?.status ? <StatusBadge status={selectedDetail.status} /> : '--'} />
                      </DetailGrid>
                      <InfoBlock title="策略说明" content={detailDescription || '暂无说明'} />
                    </div>
                  ) : (
                    <div className="space-y-4">
                      <div className="grid gap-4 md:grid-cols-2">
                        <Field label="策略名称">
                          <Input
                            value={draft?.name || ''}
                            onChange={(event) => updateDraftField('name', event.target.value)}
                            placeholder="例如：趋势跟踪策略 1"
                          />
                        </Field>
                        <Field label="状态">
                          <select
                            value={draft?.status || 'draft'}
                            onChange={(event) => updateDraftField('status', event.target.value)}
                            className="w-full rounded-xl border border-border bg-black px-4 py-3 text-sm text-white outline-none transition focus:border-primary"
                          >
                            {STATUS_OPTIONS.map((option) => (
                              <option key={option.value} value={option.value}>{option.label}</option>
                            ))}
                          </select>
                        </Field>
                      </div>

                      <Field label="策略说明">
                        <Textarea
                          value={draft?.description || ''}
                          onChange={(event) => updateDraftField('description', event.target.value)}
                          rows={4}
                          placeholder="简要描述这条策略的适用场景、选股思路或仓位规则。"
                        />
                      </Field>
                    </div>
                  )}
                </SectionBlock>

                <SectionBlock title="策略参数" description="参数会根据所选策略自动切换，直接填写即可。">
                  {mode === 'view' ? (
                    <div className="grid gap-4 md:grid-cols-2">
                      {(activePreset?.paramSchema || []).map((item) => (
                        <ParamValueCard
                          key={item.key}
                          label={item.label}
                          description={item.description}
                          value={selectedDetail?.default_params?.[item.key]}
                          suffix={item.type === 'number' ? '' : ''}
                        />
                      ))}
                    </div>
                  ) : (
                    <div className={`grid gap-4 ${(activePreset?.paramSchema || []).length >= 3 ? 'md:grid-cols-3' : 'md:grid-cols-2'}`}>
                      {(activePreset?.paramSchema || []).map((item) => (
                        <Field key={item.key} label={item.label}>
                          <Input
                            {...getInputAttributes(item)}
                            value={draft?.params?.[item.key] ?? ''}
                            onChange={(event) => updateParamField(item.key, event.target.value)}
                          />
                          {item.description ? <div className="text-xs leading-6 text-white/45">{item.description}</div> : null}
                        </Field>
                      ))}
                    </div>
                  )}
                </SectionBlock>

              </>
            )}

            {error ? (
              <ErrorBanner
                text={error}
                showLoginAction={errorNeedsLogin}
                onLogin={() => openAuthModal('login', '策略创建与编辑需要登录后使用。')}
              />
            ) : null}
            {success ? <SuccessBanner text={success} /> : null}
          </div>
        </section>
      </section>

      {pendingAction ? (
        <ConfirmDialog
          title="检测到未保存修改"
          description={buildPendingActionDescription(pendingAction)}
          onSaveAndContinue={handleSaveAndContinue}
          onDiscardAndContinue={handleDiscardAndContinue}
          onStay={() => setPendingAction(null)}
          saving={saving}
        />
      ) : null}

      {deleteConfirmOpen ? (
        <DeleteConfirmDialog
          strategyName={selectedDetail?.name || ''}
          deleting={deleting}
          onCancel={() => {
            if (!deleting) {
              setDeleteConfirmOpen(false);
            }
          }}
          onConfirm={handleDeleteStrategy}
        />
      ) : null}

      {aiDialogOpen ? (
        <AIGenerateDialog
          ticker={aiTicker}
          onTickerChange={setAiTicker}
          loading={aiLoading}
          result={aiResult}
          backtestLoading={aiBacktestLoading}
          backtestData={aiBacktestData}
          error={aiError}
          onGenerate={handleAIGenerate}
          onAdopt={handleAIAdopt}
          onClose={() => setAiDialogOpen(false)}
        />
      ) : null}
    </div>
  );
}

function CreateStrategyDropdown({ open, onToggle, onSelect, variant = 'primary' }) {
  const buttonClass = variant === 'primary'
    ? 'border-primary/30 bg-primary/10 text-primary hover:bg-primary/20'
    : 'border-white/10 bg-black/20 text-white/75 hover:border-white/20 hover:text-white';

  return (
    <div className="relative">
      <button
        type="button"
        onClick={onToggle}
        className={`rounded-xl border px-3 py-2 text-xs font-medium transition ${buttonClass}`}
      >
        新建策略
      </button>
      {open ? (
        <div className="absolute right-0 z-30 mt-2 w-72 rounded-2xl border border-white/10 bg-slate-950 p-2 shadow-2xl">
          <div className="px-3 py-2 text-xs uppercase tracking-[0.16em] text-white/35">选择策略</div>
          <div className="space-y-1">
            {STRATEGY_PRESETS.map((preset) => (
              <button
                key={preset.typeKey}
                type="button"
                onClick={() => onSelect(preset.typeKey)}
                className="w-full rounded-xl border border-transparent px-3 py-3 text-left transition hover:border-white/10 hover:bg-white/5"
              >
                <div className="text-sm font-medium text-white">{preset.typeLabel}</div>
                <div className="mt-1 text-xs leading-6 text-white/45">{preset.defaultDescription}</div>
              </button>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function PanelCard({ title, description, action, children }) {
  return (
    <section className="rounded-2xl border border-border bg-card p-6">
      <div className="mb-5 flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold text-white">{title}</h2>
          {description ? <p className="mt-2 text-sm leading-6 text-white/55">{description}</p> : null}
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

function SectionBlock({ title, description, children }) {
  return (
    <section className="space-y-4 rounded-2xl border border-white/5 bg-black/15 p-4 md:p-5">
      <div>
        <h3 className="text-sm font-semibold text-white">{title}</h3>
        {description ? <p className="mt-2 text-sm leading-6 text-white/50">{description}</p> : null}
      </div>
      {children}
    </section>
  );
}

function DetailGrid({ children }) {
  return <div className="grid gap-3 md:grid-cols-2">{children}</div>;
}

function DetailItem({ label, value }) {
  return (
    <div className="rounded-xl border border-white/5 bg-black/20 px-4 py-3 text-sm">
      <div className="text-white/45">{label}</div>
      <div className="mt-2 font-medium text-white/85">{value || '--'}</div>
    </div>
  );
}

function InfoBlock({ title, content }) {
  return (
    <div className="space-y-3 rounded-2xl border border-white/5 bg-black/20 p-4">
      <div className="text-sm font-medium text-white">{title}</div>
      <div className="text-sm leading-7 text-white/75">{content}</div>
    </div>
  );
}

function ParamValueCard({ label, description, value }) {
  return (
    <div className="rounded-2xl border border-white/5 bg-black/20 p-4">
      <div className="text-sm font-medium text-white">{label}</div>
      <div className="mt-3 text-2xl font-semibold text-white">{formatParamValue(value)}</div>
      {description ? <div className="mt-2 text-xs leading-6 text-white/45">{description}</div> : null}
    </div>
  );
}

function Field({ label, children }) {
  return (
    <div className="space-y-2">
      <div className="text-sm font-medium text-white/70">{label}</div>
      {children}
    </div>
  );
}

function Input(props) {
  return <input {...props} className="w-full rounded-xl border border-border bg-black px-4 py-3 text-sm text-white outline-none transition placeholder:text-white/25 focus:border-primary disabled:opacity-60" />;
}

function Textarea(props) {
  return <textarea {...props} className="w-full rounded-xl border border-border bg-black px-4 py-3 text-sm text-white outline-none transition placeholder:text-white/25 focus:border-primary" />;
}

function ReadonlyField({ value }) {
  return (
    <div className="rounded-xl border border-white/10 bg-black/20 px-4 py-3 text-sm font-medium text-white/80">
      {value || '--'}
    </div>
  );
}

function ConfirmDialog({ title, description, onSaveAndContinue, onDiscardAndContinue, onStay, saving }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 px-4 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-2xl border border-white/10 bg-slate-950 p-6 shadow-2xl">
        <div className="space-y-3">
          <div className="text-lg font-semibold text-white">{title}</div>
          <p className="text-sm leading-7 text-white/65">{description}</p>
        </div>
        <div className="mt-6 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <button
            type="button"
            onClick={onStay}
            className="rounded-xl border border-white/10 bg-black/20 px-4 py-2.5 text-sm font-medium text-white/75 transition hover:border-white/20 hover:text-white"
          >
            留在当前页
          </button>
          <button
            type="button"
            onClick={onDiscardAndContinue}
            className="rounded-xl border border-negative/30 bg-negative/10 px-4 py-2.5 text-sm font-medium text-red-200 transition hover:bg-negative/20"
          >
            放弃修改
          </button>
          <button
            type="button"
            onClick={onSaveAndContinue}
            disabled={saving}
            className="rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-orange-500 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {saving ? '保存中...' : '保存并切换'}
          </button>
        </div>
      </div>
    </div>
  );
}

function DeleteConfirmDialog({ strategyName, deleting, onCancel, onConfirm }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 px-4 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-2xl border border-white/10 bg-slate-950 p-6 shadow-2xl">
        <div className="space-y-3">
          <div className="text-lg font-semibold text-white">确认删除策略</div>
          <p className="text-sm leading-7 text-white/65">
            确认删除“{strategyName || '当前策略'}”？删除后不可恢复。
          </p>
          <p className="text-xs leading-6 text-white/45">
            若该策略仍被股票信号配置引用，系统会阻止删除并提示你先替换引用。
          </p>
        </div>
        <div className="mt-6 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <button
            type="button"
            onClick={onCancel}
            disabled={deleting}
            className="rounded-xl border border-white/10 bg-black/20 px-4 py-2.5 text-sm font-medium text-white/75 transition hover:border-white/20 hover:text-white disabled:cursor-not-allowed disabled:opacity-60"
          >
            取消
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={deleting}
            className="rounded-xl border border-negative/40 bg-negative/10 px-4 py-2.5 text-sm font-semibold text-red-200 transition hover:bg-negative/20 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {deleting ? '删除中...' : '确认删除'}
          </button>
        </div>
      </div>
    </div>
  );
}

function EmptyState({ text }) {
  return (
    <div className="rounded-2xl border border-dashed border-border bg-black/20 px-6 py-14 text-center text-sm text-white/45">
      {text}
    </div>
  );
}

function StatusBadge({ status }) {
  const map = {
    draft: 'bg-white/10 text-white/70 border-white/10',
    active: 'bg-positive/10 text-green-200 border-positive/20',
    archived: 'bg-negative/10 text-red-200 border-negative/20',
  };
  const labelMap = {
    draft: '草稿',
    active: '启用',
    archived: '归档',
  };

  return (
    <span className={`inline-flex rounded-full border px-2.5 py-1 text-xs font-medium ${map[status] || map.draft}`}>
      {labelMap[status] || status}
    </span>
  );
}

function MiniStat({ label, value }) {
  return (
    <div className="rounded-2xl border border-white/10 bg-black/20 px-4 py-3">
      <div className="text-xs uppercase tracking-[0.16em] text-white/35">{label}</div>
      <div className="mt-2 text-sm font-medium text-white/85">{value}</div>
    </div>
  );
}

function ErrorBanner({ text, showLoginAction = false, onLogin }) {
  return (
    <div className="rounded-xl border border-negative/40 bg-negative/10 px-4 py-3 text-sm text-red-200">
      <div>{text}</div>
      {showLoginAction ? (
        <button
          type="button"
          onClick={onLogin}
          className="mt-2 inline-flex rounded-lg border border-rose-300/40 px-2.5 py-1 text-xs text-rose-100 transition hover:bg-rose-500/15"
        >
          去登录
        </button>
      ) : null}
    </div>
  );
}

function SuccessBanner({ text }) {
  return <div className="rounded-xl border border-positive/40 bg-positive/10 px-4 py-3 text-sm text-green-200">{text}</div>;
}

function shouldConfirmBeforeAction(action, context) {
  const { isDirty, mode, selectedId } = context;
  if (!isDirty || (mode !== 'edit' && mode !== 'create')) {
    return false;
  }

  if (action?.type === 'select' && action.strategyId === selectedId && mode !== 'create') {
    return false;
  }

  return action?.type === 'select' || action?.type === 'create';
}

function buildPendingActionDescription(action) {
  if (action?.type === 'create') {
    return '当前工作区还有未保存内容。是否先保存当前修改，再新建策略？';
  }

  return '当前工作区还有未保存内容。是否先保存当前修改，再切换到另一条策略？';
}

function validateDraft(draft, definition, preset) {
  if (!draft?.name?.trim()) {
    return '策略名称不能为空。';
  }

  if (!preset) {
    return '未识别的策略配置。';
  }

  return validateStrategyParams(definition, draft.params || {});
}

function areDraftsEqual(left, right) {
  return JSON.stringify(left || {}) === JSON.stringify(right || {});
}

function formatDateTime(value) {
  if (!value) return '--';
  return value.replace('T', ' ').replace('Z', '');
}

function formatParamValue(value) {
  if (value === null || value === undefined || value === '') return '--';
  if (typeof value === 'number') {
    return Number.isInteger(value) ? String(value) : String(Number(value).toFixed(4)).replace(/0+$/, '').replace(/\.$/, '');
  }
  return String(value);
}

const CONFIDENCE_META = {
  high: { label: '高', color: 'text-emerald-300 border-emerald-400/40 bg-emerald-500/10' },
  medium: { label: '中', color: 'text-amber-300 border-amber-400/40 bg-amber-500/10' },
  low: { label: '低', color: 'text-rose-300 border-rose-400/40 bg-rose-500/10' },
};

function AIGenerateDialog({ ticker, onTickerChange, loading, result, backtestLoading, backtestData, error, onGenerate, onAdopt, onClose }) {
  const rec = result?.recommendation || null;
  const summary = rec?.market_summary || null;
  const btPreview = backtestData?.backtest_preview || null;
  const btError = backtestData?.backtest_error || null;
  const iterations = backtestData?.iterations || [];
  const confidenceMeta = CONFIDENCE_META[rec?.confidence] || CONFIDENCE_META.medium;
  const [showIterations, setShowIterations] = useState(false);

  const hasResult = Boolean(rec);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 px-4 backdrop-blur-md">
      <div className="w-full max-w-lg max-h-[90vh] overflow-y-auto rounded-2xl border border-white/25 bg-[#121620]/95 p-6 shadow-[0_8px_48px_rgba(0,0,0,0.6)] ring-1 ring-white/10">
        {!hasResult ? (
          <>
            <div className="space-y-3">
              <div className="text-lg font-semibold text-white">✨ AI 智能生成策略</div>
              <p className="text-sm leading-7 text-white/65">
                输入一只股票代码，AI 会分析该股票近期走势，推荐最合适的策略类型和参数，并用近 6 个月数据自动回测验证。
              </p>
            </div>
            <div className="mt-4">
              <input
                value={ticker}
                onChange={(e) => onTickerChange(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && !loading && onGenerate()}
                placeholder="输入股票代码，如 600519 或 00700"
                className="w-full rounded-xl border border-border bg-black px-4 py-3 text-sm text-white outline-none transition placeholder:text-white/25 focus:border-primary"
                autoFocus
              />
            </div>
            {error ? <div className="mt-3 rounded-lg border border-rose-400/40 bg-rose-500/10 px-3 py-2 text-xs text-rose-200">{error}</div> : null}
            <div className="mt-5 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
              <button
                type="button"
                onClick={onClose}
                className="rounded-xl border border-white/10 bg-black/20 px-4 py-2.5 text-sm font-medium text-white/75 transition hover:border-white/20 hover:text-white"
              >
                取消
              </button>
              <button
                type="button"
                onClick={onGenerate}
                disabled={loading || !ticker.trim()}
                className="rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-orange-500 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {loading ? '分析中（含回测验证）...' : '开始分析'}
              </button>
            </div>
          </>
        ) : (
          <>
            <div className="space-y-3">
              <div className="text-lg font-semibold text-white">✨ AI 策略匹配结果</div>
              {summary ? (
                <div className="flex flex-wrap items-center gap-2 text-xs text-white/55">
                  <span className="font-medium text-white/80">{summary.name}（{summary.ticker}）</span>
                  <span>最新价 {summary.price}</span>
                </div>
              ) : null}
            </div>

            {summary ? (
              <div className="mt-3 grid grid-cols-3 gap-2">
                <MiniInfo label="60日涨跌幅" value={`${summary.change_pct_60d >= 0 ? '+' : ''}${summary.change_pct_60d?.toFixed(1)}%`} accent={summary.change_pct_60d >= 0 ? 'up' : 'down'} />
                <MiniInfo label="波动率" value={`${summary.volatility_20d?.toFixed(1)}%`} />
                <MiniInfo label="均量比" value={summary.volume_ma5_to_ma20?.toFixed(2)} />
                <MiniInfo label="RSI" value={summary.rsi14?.toFixed(1)} />
                <MiniInfo label="MACD柱" value={summary.macd_histogram?.toFixed(4)} accent={summary.macd_histogram >= 0 ? 'up' : 'down'} />
                <MiniInfo label="均线" value={summary.ma_status} />
              </div>
            ) : null}

            <div className="mt-4 space-y-3 rounded-xl border border-white/10 bg-black/20 p-4">
              <div className="flex items-center gap-2">
                <span className="text-sm font-semibold text-white">🎯 {rec.strategy_label}</span>
                <span className={`inline-flex rounded-full border px-2 py-0.5 text-[11px] font-medium ${confidenceMeta.color}`}>
                  置信度：{confidenceMeta.label}
                </span>
              </div>
              <p className="text-sm leading-7 text-white/70">{rec.reason}</p>
              {rec.params ? (
                <div className="flex flex-wrap gap-2">
                  {Object.entries(rec.params).map(([key, value]) => (
                    <span key={key} className="rounded-full border border-white/10 bg-white/5 px-2.5 py-1 text-[11px] text-white/55">
                      {key}={String(value)}
                    </span>
                  ))}
                </div>
              ) : null}
            </div>

            {backtestLoading ? (
              <div className="mt-4 space-y-2">
                <div className="text-sm font-semibold text-white">📊 近 6 个月回测验证</div>
                <div className="flex items-center gap-2 rounded-xl border border-white/10 bg-black/20 px-4 py-3">
                  <span className="inline-block h-3 w-3 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                  <span className="text-xs text-white/55">正在用近 6 个月历史数据回测验证 + 迭代优化中...</span>
                </div>
              </div>
            ) : btError ? (
              <div className="mt-4 space-y-2">
                <div className="text-sm font-semibold text-white">📊 近 6 个月回测验证</div>
                <div className="rounded-lg border border-amber-400/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
                  ⚠️ {btError}
                </div>
              </div>
            ) : btPreview ? (
              <div className="mt-4 space-y-2">
                <div className="text-sm font-semibold text-white">📊 近 6 个月回测验证</div>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                  <BacktestMiniCard label="总收益" value={fmtPct(btPreview.total_return)} accent={btPreview.total_return >= 0 ? 'up' : 'down'} />
                  <BacktestMiniCard label="最大回撤" value={fmtPct(btPreview.max_drawdown)} accent="down" />
                  <BacktestMiniCard label="夏普比率" value={btPreview.sharpe_ratio?.toFixed(2)} accent={btPreview.sharpe_ratio >= 1 ? 'up' : btPreview.sharpe_ratio < 0 ? 'down' : 'normal'} />
                  <BacktestMiniCard label="交易次数" value={btPreview.trade_count ?? '--'} />
                </div>
                <div className="grid grid-cols-2 gap-2">
                  <BacktestMiniCard label="胜率" value={fmtPct(btPreview.win_rate)} />
                  <BacktestMiniCard label="年化收益" value={fmtPct(btPreview.annual_return)} accent={btPreview.annual_return >= 0 ? 'up' : 'down'} />
                </div>
                {btPreview.total_return < 0 ? (
                  <div className="rounded-lg border border-rose-400/30 bg-rose-500/10 px-3 py-2 text-xs text-rose-200">
                    ⚠️ 该策略在近 6 个月回测中收益为负（{fmtPct(btPreview.total_return)}），建议谨慎采纳，或在回测引擎中调整参数后重新验证。
                  </div>
                ) : null}
                <div className="text-[10px] text-white/30">
                  ⓘ 回测基于近 6 个月历史数据，结果仅供参考，不代表未来收益。
                </div>
              </div>
            ) : null}

            {iterations.length > 0 ? (
              <div className="mt-4">
                <button
                  type="button"
                  onClick={() => setShowIterations(!showIterations)}
                  className="flex items-center gap-1 text-xs font-medium text-white/50 transition hover:text-white/70"
                >
                  <span>{showIterations ? '▼' : '▶'}</span>
                  <span>迭代优化过程（{iterations.length} 轮）</span>
                </button>
                {showIterations ? (
                  <div className="mt-2 space-y-2">
                    {iterations.map((iter) => (
                      <div key={iter.round} className="rounded-lg border border-white/5 bg-black/30 p-3">
                        <div className="flex items-center justify-between">
                          <span className="text-xs font-medium text-white/65">第 {iter.round} 轮</span>
                          <div className="flex items-center gap-2 text-[10px] text-white/40">
                            <span>收益 {fmtPct(iter.backtest_preview?.total_return)}</span>
                            <span>夏普 {iter.backtest_preview?.sharpe_ratio?.toFixed(2)}</span>
                            <span>回撤 {fmtPct(iter.backtest_preview?.max_drawdown)}</span>
                          </div>
                        </div>
                        {iter.params ? (
                          <div className="mt-1.5 flex flex-wrap gap-1.5">
                            {Object.entries(iter.params).map(([k, v]) => (
                              <span key={k} className="rounded-full border border-white/5 bg-white/5 px-2 py-0.5 text-[10px] text-white/45">
                                {k}={String(v)}
                              </span>
                            ))}
                          </div>
                        ) : null}
                        {iter.adjustment ? (
                          <div className="mt-1.5 text-[11px] leading-5 text-white/50">💡 {iter.adjustment}</div>
                        ) : null}
                      </div>
                    ))}
                  </div>
                ) : null}
              </div>
            ) : null}

            {rec.confidence === 'low' ? (
              <div className="mt-3 rounded-lg border border-amber-400/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
                ⚠️ AI 对该推荐的置信度较低。该股票的行情特征不太典型，推荐结果仅供参考。建议在回测引擎中先验证策略效果，或等市场走势更加明朗后重新分析。
              </div>
            ) : null}

            {error ? <div className="mt-3 rounded-lg border border-rose-400/40 bg-rose-500/10 px-3 py-2 text-xs text-rose-200">{error}</div> : null}

            <div className="mt-5 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
              <button
                type="button"
                onClick={() => { onTickerChange(ticker); onClose(); }}
                className="rounded-xl border border-white/10 bg-black/20 px-4 py-2.5 text-sm font-medium text-white/75 transition hover:border-white/20 hover:text-white"
              >
                关闭
              </button>
              <button
                type="button"
                onClick={() => { onTickerChange(ticker); onGenerate(); }}
                disabled={loading}
                className="rounded-xl border border-primary/30 bg-primary/10 px-4 py-2.5 text-sm font-medium text-primary transition hover:bg-primary/20 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {loading ? '分析中...' : '重新分析'}
              </button>
              <button
                type="button"
                onClick={onAdopt}
                className="rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-orange-500"
              >
                采纳并创建策略
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function BacktestMiniCard({ label, value, accent }) {
  const color = accent === 'up' ? 'text-rose-300' : accent === 'down' ? 'text-emerald-300' : 'text-white/80';
  return (
    <div className="rounded-lg border border-white/5 bg-black/30 px-2.5 py-1.5">
      <div className="text-[10px] text-white/40">{label}</div>
      <div className={`mt-0.5 text-xs font-medium ${color}`}>{value ?? '--'}</div>
    </div>
  );
}

function fmtPct(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--';
  return `${(Number(value) * 100).toFixed(2)}%`;
}

function MiniInfo({ label, value, accent }) {
  const color = accent === 'up' ? 'text-rose-300' : accent === 'down' ? 'text-emerald-300' : 'text-white/80';
  return (
    <div className="rounded-lg border border-white/5 bg-black/30 px-2.5 py-1.5">
      <div className="text-[10px] text-white/40">{label}</div>
      <div className={`mt-0.5 text-xs font-medium ${color}`}>{value ?? '--'}</div>
    </div>
  );
}
