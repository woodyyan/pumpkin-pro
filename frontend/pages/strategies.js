import { useEffect, useMemo, useState } from 'react';

import { requestJson } from '../lib/api';
import { useAuth } from '../lib/auth-context';
import { isAuthRequiredError } from '../lib/auth-storage';
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

  const showWorkspaceEmpty = mode !== 'create' && !selectedDetail && !loadingDetail;

  return (
    <div className="max-w-7xl mx-auto space-y-6 pb-12">
      <section className="bg-card border border-border rounded-2xl p-6 md:p-8">
        <div className="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
          <div className="space-y-3 max-w-3xl">
            <span className="inline-flex items-center rounded-full border border-primary/30 bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
              Pumpkin Pro · 策略库
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
            <CreateStrategyDropdown
              open={createMenuOpen}
              onToggle={() => setCreateMenuOpen((prev) => !prev)}
              onSelect={(strategyType) => requestWorkspaceAction({ type: 'create', strategyType })}
            />
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
