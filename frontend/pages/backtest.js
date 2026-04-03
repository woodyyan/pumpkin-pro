import { useEffect, useMemo, useRef, useState, useCallback } from 'react';

import { requestJson } from '../lib/api';
import { useAuth } from '../lib/auth-context';
import { getStrategyPresetByImplementation } from '../lib/strategy-presets';

const DATA_SOURCE_OPTIONS = [
  { value: 'online', label: '在线下载', description: '下载 A 股 / 港股历史数据' },
  { value: 'csv', label: '本地 CSV', description: '上传本地股票 CSV 文件回测' },
  { value: 'sample', label: '示例行情', description: '自动生成示例历史行情数据' },
];

function formatDateInputValue(date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function buildDefaultDateRange() {
  const endDate = new Date();
  endDate.setHours(0, 0, 0, 0);
  endDate.setDate(endDate.getDate() - 1);

  const startDate = new Date(endDate);
  startDate.setFullYear(startDate.getFullYear() - 1);

  return {
    startDate: formatDateInputValue(startDate),
    endDate: formatDateInputValue(endDate),
  };
}

const defaultDateRange = buildDefaultDateRange();

const DEFAULT_FORM = {
  dataSource: 'online',
  ticker: '600519',
  startDate: defaultDateRange.startDate,
  endDate: defaultDateRange.endDate,
  capital: 100000,
  feePct: 0.001,
  strategyId: '',
  csvContent: '',
  csvFilename: '',
  sampleConfig: {
    start_price: 100,
    drift: 0.0005,
    volatility: 0.02,
    seed: 42,
  },
};

const ERROR_FIELD_LABELS = {
  ticker: '股票代码',
  capital: '初始资金',
  fee_pct: '手续费率',
  strategy_params: '策略参数',
};

export default function BacktestPage() {
  const { isLoggedIn, openAuthModal } = useAuth();
  const [form, setForm] = useState(DEFAULT_FORM);
  const [strategies, setStrategies] = useState([]);
  const [strategiesLoading, setStrategiesLoading] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState(null);

  // ── History runs ──
  const [historyRuns, setHistoryRuns] = useState([]);
  const [historyTotal, setHistoryTotal] = useState(0);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [activeRunId, setActiveRunId] = useState(null);
  const [historyExpanded, setHistoryExpanded] = useState(true);
  const [priceChartLegend, setPriceChartLegend] = useState([]);

  const priceChartContainerRef = useRef(null);
  const equityChartContainerRef = useRef(null);
  const auxChartContainerRef = useRef(null);
  const priceChartRef = useRef(null);
  const equityChartRef = useRef(null);
  const auxChartRef = useRef(null);

  const selectedStrategy = useMemo(
    () => strategies.find((item) => item.id === form.strategyId) || null,
    [form.strategyId, strategies],
  );

  const fetchHistory = useCallback(async () => {
    if (!isLoggedIn) return;
    setHistoryLoading(true);
    try {
      const data = await requestJson('/api/backtest/runs?limit=20', undefined, '加载回测历史失败');
      setHistoryRuns(data?.items || []);
      setHistoryTotal(data?.total || 0);
    } catch {
      // silently fail - history is not critical
    } finally {
      setHistoryLoading(false);
    }
  }, [isLoggedIn]);

  useEffect(() => {
    fetchHistory();
  }, [fetchHistory]);

  const resultStrategyParams = result?.strategy?.params || {};

  const metricCards = useMemo(() => {
    if (!result?.metrics) return [];

    return [
      { title: '总收益率', value: result.metrics.total_return_pct, type: 'percent' },
      { title: '买入并持有收益', value: result.metrics.buy_and_hold_return_pct, type: 'percent' },
      { title: '超额收益（策略-买入持有）', value: result.metrics.excess_return_pct, type: 'percent' },
      { title: '年化收益率', value: result.metrics.annual_return_pct, type: 'percent' },
      { title: '最大回撤', value: -Math.abs(result.metrics.max_drawdown_pct || 0), type: 'percent' },
      { title: '夏普比率', value: result.metrics.sharpe_ratio, type: 'number' },
      { title: '胜率', value: result.metrics.win_rate_pct, type: 'percent' },
      { title: '交易次数', value: result.metrics.total_trades, type: 'integer' },
      { title: '最终资产', value: result.metrics.final_capital, type: 'currency' },
      { title: '总手续费', value: result.metrics.total_fee, type: 'currency' },
    ];
  }, [result]);

  useEffect(() => {
    const loadStrategies = async () => {
      setStrategiesLoading(true);
      try {
        const data = await requestJson('/api/strategies/active', undefined, '加载可用策略失败');
        const items = data?.items || [];
        setStrategies(items);
        setForm((prev) => {
          const currentStrategy = items.find((item) => item.id === prev.strategyId) || items[0] || null;
          if (!currentStrategy) {
            return { ...prev, strategyId: '' };
          }

          return {
            ...prev,
            strategyId: currentStrategy.id,
          };
        });
      } catch (err) {
        setError(err.message || '加载策略失败');
      } finally {
        setStrategiesLoading(false);
      }
    };

    loadStrategies();
  }, []);

  useEffect(() => {
    let cleanup = () => {};

    const renderCharts = async () => {
      if (!result?.kline_data?.length) {
        destroyCharts();
        setPriceChartLegend([]);
        return;
      }

      const { createChart, ColorType } = await import('lightweight-charts');
      destroyCharts();
      setPriceChartLegend([]);

      const resizeHandlers = [];
      const registerResize = (chart, container) => {
        const resize = () => {
          if (!container || !chart) return;
          chart.applyOptions({ width: container.clientWidth || 400 });
          chart.timeScale().fitContent();
        };
        window.addEventListener('resize', resize);
        resizeHandlers.push(resize);
      };

      if (priceChartContainerRef.current) {
        const priceChart = createChart(priceChartContainerRef.current, buildChartOptions(priceChartContainerRef.current.clientWidth, 430, ColorType));
        const candleSeries = priceChart.addCandlestickSeries({
          upColor: '#22c55e',
          downColor: '#ef4444',
          borderVisible: false,
          wickUpColor: '#22c55e',
          wickDownColor: '#ef4444',
        });

        candleSeries.setData(
          result.kline_data.map((item) => ({
            time: item.date,
            open: item.open,
            high: item.high,
            low: item.low,
            close: item.close,
          })),
        );

        const markers = (result.trades || [])
          .filter((trade) => trade?.date)
          .map((trade) => ({
            time: trade.date,
            position: trade.type === 'buy' ? 'belowBar' : 'aboveBar',
            color: trade.type === 'buy' ? '#22c55e' : '#ef4444',
            shape: trade.type === 'buy' ? 'arrowUp' : 'arrowDown',
            text: trade.type === 'buy' ? 'BUY' : 'SELL',
          }))
          .sort((a, b) => a.time.localeCompare(b.time));
        candleSeries.setMarkers(markers);

        const seriesSample = result.kline_data[0] || {};
        const maKeys = Object.keys(seriesSample).filter((key) => /^MA\d+$/.test(key)).sort((a, b) => Number(a.slice(2)) - Number(b.slice(2)));
        const maColors = ['#f59e0b', '#8b5cf6', '#38bdf8'];
        const legendItems = [];
        maKeys.forEach((key, index) => {
          const color = maColors[index % maColors.length];
          const lineSeries = priceChart.addLineSeries({
            color,
            lineWidth: 2,
            title: '',
          });
          lineSeries.setData(
            result.kline_data
              .filter((item) => item[key] !== null && item[key] !== undefined)
              .map((item) => ({ time: item.date, value: item[key] })),
          );
          legendItems.push({ label: key, color });
        });

        const bollingerKeys = ['BB_upper', 'BB_mid', 'BB_lower'].filter((key) => Object.prototype.hasOwnProperty.call(seriesSample, key));
        const bollingerStyles = {
          BB_upper: { color: '#38bdf8', lineWidth: 1 },
          BB_mid: { color: '#f59e0b', lineWidth: 1 },
          BB_lower: { color: '#38bdf8', lineWidth: 1 },
        };
        const bollingerLabels = { BB_upper: '布林上轨', BB_mid: '布林中轨', BB_lower: '布林下轨' };
        bollingerKeys.forEach((key) => {
          const style = bollingerStyles[key];
          const lineSeries = priceChart.addLineSeries({ title: '', ...style });
          lineSeries.setData(
            result.kline_data
              .filter((item) => item[key] !== null && item[key] !== undefined)
              .map((item) => ({ time: item.date, value: item[key] })),
          );
          legendItems.push({ label: bollingerLabels[key] || key, color: style.color });
        });
        setPriceChartLegend(legendItems);

        priceChart.timeScale().fitContent();
        priceChartRef.current = priceChart;
        registerResize(priceChart, priceChartContainerRef.current);
      }

      if (equityChartContainerRef.current && result.analysis?.equity_curve?.length) {
        const equityChart = createChart(equityChartContainerRef.current, buildChartOptions(equityChartContainerRef.current.clientWidth, 240, ColorType));
        const strategyEquitySeries = equityChart.addAreaSeries({
          lineColor: '#e67e22',
          topColor: 'rgba(230, 126, 34, 0.35)',
          bottomColor: 'rgba(230, 126, 34, 0.03)',
          lineWidth: 2,
        });
        strategyEquitySeries.setData(
          result.analysis.equity_curve.map((item) => ({
            time: item.date,
            value: item.portfolio_value,
          })),
        );

        if (result.analysis?.buy_and_hold_curve?.length) {
          const buyHoldSeries = equityChart.addLineSeries({
            color: '#38bdf8',
            lineWidth: 2,
            title: '买入并持有',
          });
          buyHoldSeries.setData(
            result.analysis.buy_and_hold_curve.map((item) => ({
              time: item.date,
              value: item.portfolio_value,
            })),
          );
        }

        equityChart.timeScale().fitContent();
        equityChartRef.current = equityChart;
        registerResize(equityChart, equityChartContainerRef.current);
      }

      if (auxChartContainerRef.current) {
        const auxChart = createChart(auxChartContainerRef.current, buildChartOptions(auxChartContainerRef.current.clientWidth, 240, ColorType));
        const seriesSample = result.kline_data[0] || {};
        const rsiKey = Object.keys(seriesSample).find((key) => /^RSI_\d+$/.test(key));

        if (rsiKey) {
          const rsiSeries = auxChart.addLineSeries({ color: '#8b5cf6', lineWidth: 2, title: rsiKey });
          rsiSeries.setData(
            result.kline_data
              .filter((item) => item[rsiKey] !== null && item[rsiKey] !== undefined)
              .map((item) => ({ time: item.date, value: item[rsiKey] })),
          );
          rsiSeries.createPriceLine({ price: Number(resultStrategyParams.rsi_low), color: '#22c55e', lineStyle: 2, axisLabelVisible: true, title: '低位线' });
          rsiSeries.createPriceLine({ price: Number(resultStrategyParams.rsi_high), color: '#ef4444', lineStyle: 2, axisLabelVisible: true, title: '高位线' });
        } else if (result.analysis?.drawdown_curve?.length) {
          const drawdownSeries = auxChart.addAreaSeries({
            lineColor: '#ef4444',
            topColor: 'rgba(239, 68, 68, 0.15)',
            bottomColor: 'rgba(239, 68, 68, 0.02)',
            lineWidth: 2,
          });
          drawdownSeries.setData(
            result.analysis.drawdown_curve.map((item) => ({
              time: item.date,
              value: item.drawdown_pct,
            })),
          );
        }

        auxChart.timeScale().fitContent();
        auxChartRef.current = auxChart;
        registerResize(auxChart, auxChartContainerRef.current);
      }

      cleanup = () => {
        resizeHandlers.forEach((handler) => window.removeEventListener('resize', handler));
        destroyCharts();
      };
    };

    renderCharts();

    return () => cleanup();
  }, [result, resultStrategyParams.rsi_high, resultStrategyParams.rsi_low]);

  const destroyCharts = () => {
    [priceChartRef, equityChartRef, auxChartRef].forEach((chartRef) => {
      if (chartRef.current) {
        chartRef.current.remove();
        chartRef.current = null;
      }
    });
  };

  const updateField = (key, value) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  const selectStrategy = (strategyId) => {
    const strategy = strategies.find((item) => item.id === strategyId);
    if (!strategy) return;

    setForm((prev) => ({
      ...prev,
      strategyId: strategy.id,
    }));
  };

  const updateSampleConfig = (key, value) => {
    setForm((prev) => ({
      ...prev,
      sampleConfig: {
        ...prev.sampleConfig,
        [key]: value,
      },
    }));
  };

  const handleFileUpload = async (event) => {
    const file = event.target.files?.[0];
    if (!file) return;

    const content = await file.text();
    setForm((prev) => ({
      ...prev,
      csvFilename: file.name,
      csvContent: content,
    }));
    setError('');
  };

  const runBacktest = async () => {
    setError('');

    if (strategiesLoading) {
      setError('策略列表仍在加载，请稍后再试。');
      return;
    }

    if (!selectedStrategy) {
      setError('当前没有可用策略，请先到策略库创建并启用策略。');
      return;
    }

    if (form.dataSource === 'online' && !form.ticker.trim()) {
      setError('在线下载模式需要填写股票代码，A 股为 6 位数字，港股为 5 位数字。');
      return;
    }

    if (form.dataSource === 'csv' && !form.csvContent) {
      setError('请先上传本地 CSV 文件。');
      return;
    }

    setLoading(true);
    try {
      const payload = buildPayload(form, selectedStrategy);
      const responseData = await requestJson(
        '/api/backtest',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        },
        '回测失败，请稍后重试。',
      );

      setResult(responseData);
      setActiveRunId(null);
      // Refresh history list after a short delay (give backend async save time)
      if (isLoggedIn) {
        setTimeout(() => fetchHistory(), 800);
      }
    } catch (err) {
      setResult(null);
      setError(err.message || '回测失败，请稍后重试。');
    } finally {
      setLoading(false);
    }
  };

  const rsiKey = useMemo(() => {
    const sample = result?.kline_data?.[0];
    if (!sample) return null;
    return Object.keys(sample).find((key) => /^RSI_\d+$/.test(key)) || null;
  }, [result]);

  return (
    <div className="max-w-7xl mx-auto space-y-6 pb-12">
      <section className="bg-card border border-border rounded-2xl p-6 md:p-8">
        <div className="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
          <div className="space-y-3 max-w-3xl">
            <span className="inline-flex items-center rounded-full border border-primary/30 bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
              Wolong Pro · 历史回测
            </span>
            <div>
              <h1 className="text-3xl md:text-4xl font-semibold tracking-tight">历史回测工作台</h1>
              <p className="mt-3 text-sm md:text-base text-white/65 leading-7">
                历史回测已接入策略库。你可以直接选择已启用的策略模板，参数由策略库统一维护。
              </p>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3 text-sm text-white/70 md:min-w-[320px]">
            <MiniStat label="数据源" value="CSV / 示例 / 在线下载" />
            <MiniStat label="策略来源" value={strategiesLoading ? '加载中...' : `${strategies.length} 条启用策略`} />
            <MiniStat label="指标分析" value="收益 / 回撤 / 夏普" />
            <MiniStat label="图表输出" value="K 线 / 资产 / 回撤" />
          </div>
        </div>
      </section>

      <section className="grid gap-6 xl:grid-cols-[1.15fr_0.85fr]">
        <SectionCard title="回测配置" description="先选择数据来源，再配置回测区间与资金参数。">
          <div className="space-y-6">
            <div className="grid gap-3 md:grid-cols-3">
              {DATA_SOURCE_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => updateField('dataSource', option.value)}
                  className={`rounded-2xl border p-4 text-left transition ${
                    form.dataSource === option.value
                      ? 'border-primary bg-primary/10 shadow-[0_0_0_1px_rgba(230,126,34,0.25)]'
                      : 'border-border bg-black/20 hover:border-white/20'
                  }`}
                >
                  <div className="text-sm font-medium text-white">{option.label}</div>
                  <div className="mt-2 text-xs leading-6 text-white/60">{option.description}</div>
                </button>
              ))}
            </div>

            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
              <Field label="开始日期">
                <Input type="date" value={form.startDate} onChange={(event) => updateField('startDate', event.target.value)} />
              </Field>
              <Field label="结束日期">
                <Input type="date" value={form.endDate} onChange={(event) => updateField('endDate', event.target.value)} />
              </Field>
              <Field label="初始资金（元）">
                <Input type="number" min="1000" step="1000" value={form.capital} onChange={(event) => updateField('capital', Number(event.target.value))} />
              </Field>
              <Field label="手续费率">
                <Input type="number" min="0" max="0.05" step="0.0001" value={form.feePct} onChange={(event) => updateField('feePct', Number(event.target.value))} />
              </Field>
            </div>

            {form.dataSource === 'online' && (
              <div className="grid gap-4 md:grid-cols-2">
                <Field label="股票代码">
                  <Input
                    type="text"
                    placeholder="A 股如 600519，港股如 00700"
                    value={form.ticker}
                    onChange={(event) => updateField('ticker', event.target.value)}
                  />
                </Field>
                <Field label="说明">
                  <div className="rounded-xl border border-dashed border-border bg-black/20 px-4 py-3 text-sm leading-6 text-white/60">
                    自动识别市场：6 位数字按 A 股处理，5 位数字按港股处理。
                  </div>
                </Field>
              </div>
            )}

            {form.dataSource === 'csv' && (
              <div className="grid gap-4 md:grid-cols-2">
                <Field label="上传本地 CSV">
                  <label className="flex min-h-[108px] cursor-pointer flex-col items-center justify-center rounded-2xl border border-dashed border-border bg-black/20 px-4 text-center transition hover:border-primary/40">
                    <span className="text-sm font-medium text-white">点击选择 CSV 文件</span>
                    <span className="mt-2 text-xs leading-6 text-white/50">需要包含 date / open / high / low / close / volume 列，支持中英文列名。</span>
                    <input type="file" accept=".csv,text/csv" className="hidden" onChange={handleFileUpload} />
                  </label>
                </Field>
                <Field label="文件状态">
                  <div className="rounded-2xl border border-border bg-black/20 px-4 py-4 text-sm leading-7 text-white/70">
                    <div>
                      <span className="text-white/50">当前文件：</span>
                      <span className="text-white">{form.csvFilename || '未上传'}</span>
                    </div>
                    <div>
                      <span className="text-white/50">解析模式：</span>
                      <span>按所选日期区间截取后回测</span>
                    </div>
                    <div>
                      <span className="text-white/50">兼容列：</span>
                      <span>日期/Date、开盘/Open、收盘/Close 等</span>
                    </div>
                  </div>
                </Field>
              </div>
            )}

            {form.dataSource === 'sample' && (
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                <Field label="起始价格">
                  <Input type="number" min="1" step="1" value={form.sampleConfig.start_price} onChange={(event) => updateSampleConfig('start_price', Number(event.target.value))} />
                </Field>
                <Field label="日漂移率">
                  <Input type="number" step="0.0001" value={form.sampleConfig.drift} onChange={(event) => updateSampleConfig('drift', Number(event.target.value))} />
                </Field>
                <Field label="波动率">
                  <Input type="number" min="0.001" step="0.001" value={form.sampleConfig.volatility} onChange={(event) => updateSampleConfig('volatility', Number(event.target.value))} />
                </Field>
                <Field label="随机种子">
                  <Input type="number" min="0" step="1" value={form.sampleConfig.seed} onChange={(event) => updateSampleConfig('seed', Number(event.target.value))} />
                </Field>
              </div>
            )}
          </div>
        </SectionCard>

        <SectionCard title="策略设置">
          <div className="space-y-6">
            <Field label="回测策略">
              <select
                value={form.strategyId}
                onChange={(event) => selectStrategy(event.target.value)}
                className="w-full rounded-xl border border-border bg-black px-4 py-3 text-sm text-white outline-none transition focus:border-primary"
                disabled={strategiesLoading || strategies.length === 0}
              >
                {strategiesLoading ? (
                  <option value="">策略加载中...</option>
                ) : strategies.length === 0 ? (
                  <option value="">暂无启用策略</option>
                ) : (
                  strategies.map((strategy) => (
                    <option key={strategy.id} value={strategy.id}>
                      {strategy.name}
                    </option>
                  ))
                )}
              </select>
            </Field>

            <div className="rounded-2xl border border-border bg-black/20 p-4">
              <div className="mb-3 text-sm font-medium text-white">策略参数（只读）</div>
              {selectedStrategy ? (() => {
                const preset = getStrategyPresetByImplementation(selectedStrategy.implementation_key)
                const paramSchema = preset?.paramSchema || selectedStrategy.param_schema || []
                const defaultParams = selectedStrategy.default_params || {}
                if (paramSchema.length === 0) {
                  return <div className="text-sm text-white/50">该策略暂无可配置参数。</div>
                }
                return (
                  <div className={`grid gap-3 ${paramSchema.length >= 3 ? 'md:grid-cols-3' : 'md:grid-cols-2'}`}>
                    {paramSchema.map((item) => (
                      <div key={item.key} className="rounded-xl border border-white/5 bg-black/20 p-3">
                        <div className="text-xs text-white/50">{item.label}</div>
                        <div className="mt-1.5 text-lg font-semibold text-white">{formatParamDisplay(defaultParams[item.key])}</div>
                        {item.description ? <div className="mt-1 text-[11px] leading-5 text-white/35">{item.description}</div> : null}
                      </div>
                    ))}
                  </div>
                )
              })() : (
                <div className="text-sm text-white/50">请先选择策略。</div>
              )}
            </div>

            <div className="rounded-2xl border border-border bg-black/20 p-4 text-sm leading-7 text-white/65">
              <div className="font-medium text-white">已选策略说明</div>
              <div className="mt-2">{selectedStrategy?.description || '请先到策略库创建并启用策略。'}</div>
            </div>

            <button
              type="button"
              onClick={runBacktest}
              disabled={loading || strategiesLoading || !selectedStrategy}
              className="inline-flex w-full items-center justify-center rounded-xl bg-primary px-5 py-3 text-sm font-semibold text-white transition hover:bg-orange-500 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {loading ? '回测运行中...' : '运行历史回测'}
            </button>

            {error && <div className="rounded-xl border border-negative/40 bg-negative/10 px-4 py-3 text-sm text-red-200">{error}</div>}
          </div>
        </SectionCard>
      </section>

      {/* ── 历史运行面板 ── */}
      {isLoggedIn && (
        <section className="rounded-2xl border border-border bg-card p-6">
          <div className="mb-4 flex items-center justify-between">
            <button
              type="button"
              onClick={() => setHistoryExpanded((prev) => !prev)}
              className="flex items-center gap-2 text-lg font-semibold text-white hover:text-primary transition"
            >
              <span className={`inline-block transition-transform ${historyExpanded ? 'rotate-90' : ''}`}>▸</span>
              历史运行
              {historyTotal > 0 && <span className="text-sm font-normal text-white/40">({historyTotal})</span>}
            </button>
            {historyRuns.length > 0 && (
              <button
                type="button"
                onClick={fetchHistory}
                disabled={historyLoading}
                className="text-xs text-white/40 hover:text-white/70 transition disabled:opacity-50"
              >
                {historyLoading ? '刷新中...' : '刷新'}
              </button>
            )}
          </div>
          {historyExpanded && (
            <>
              {historyLoading && historyRuns.length === 0 ? (
                <div className="py-8 text-center text-sm text-white/40">加载中...</div>
              ) : historyRuns.length === 0 ? (
                <div className="rounded-xl border border-dashed border-border bg-black/20 px-4 py-8 text-center text-sm text-white/40">
                  运行回测后，结果会自动保存在这里。
                </div>
              ) : (
                <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                  {historyRuns.map((run) => (
                    <HistoryRunCard
                      key={run.id}
                      run={run}
                      isActive={activeRunId === run.id}
                      onSelect={async () => {
                        if (activeRunId === run.id) return;
                        setActiveRunId(run.id);
                        setLoading(true);
                        setError('');
                        try {
                          const detail = await requestJson(`/api/backtest/runs/${run.id}`, undefined, '加载回测结果失败');
                          setResult(detail?.result || null);
                        } catch (err) {
                          setError(err.message || '加载回测结果失败');
                        } finally {
                          setLoading(false);
                        }
                      }}
                      onDelete={async () => {
                        if (!confirm('确定删除这条回测记录？')) return;
                        try {
                          await requestJson(`/api/backtest/runs/${run.id}`, { method: 'DELETE' }, '删除失败');
                          if (activeRunId === run.id) {
                            setActiveRunId(null);
                            setResult(null);
                          }
                          fetchHistory();
                        } catch (err) {
                          setError(err.message || '删除失败');
                        }
                      }}
                    />
                  ))}
                </div>
              )}
            </>
          )}
        </section>
      )}

      {!isLoggedIn && (
        <div className="rounded-2xl border border-dashed border-border bg-card/60 px-6 py-6 text-center">
          <span className="text-sm text-white/45">
            <button type="button" onClick={() => openAuthModal('login', '登录后可保存和查看回测历史记录。')} className="text-primary hover:underline">登录</button>
            {' '}后可保存和查看回测历史记录
          </span>
        </div>
      )}

      {!result && !loading && (
        <div className="rounded-2xl border border-dashed border-border bg-card/60 px-6 py-16 text-center text-white/45">
          选择数据源与策略后，点击"运行历史回测"，这里会展示 K 线、资产曲线、回撤分析和交易结果。
        </div>
      )}

      {result && (
        <>
          <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
            <SummaryPill label="回测标的" value={result.data_summary?.ticker_display || result.data_summary?.ticker || '示例/CSV'} />
            <SummaryPill label="数据来源" value={result.source_used} />
            <SummaryPill label="回测区间" value={`${result.data_summary?.start_date} ~ ${result.data_summary?.end_date}`} />
            <SummaryPill label="样本数量" value={`${result.data_summary?.total_records || 0} 条`} />
            <SummaryPill label="当前策略" value={result.strategy?.name || selectedStrategy?.name || '--'} />
          </section>

          <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            {metricCards.map((metric) => (
              <MetricCard key={metric.title} title={metric.title} value={metric.value} type={metric.type} />
            ))}
          </section>

          <SectionCard title="价格走势与交易信号" description="展示 K 线、策略指标叠加以及买卖点位。">
            <div ref={priceChartContainerRef} className="h-[430px] w-full" />
            {priceChartLegend.length > 0 && (
              <div className="mt-3 flex flex-wrap gap-4 text-xs text-white/55">
                {priceChartLegend.map((item) => (
                  <span key={item.label} className="inline-flex items-center gap-2">
                    <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: item.color }} />{item.label}
                  </span>
                ))}
              </div>
            )}
          </SectionCard>

          <section className="grid gap-6 xl:grid-cols-2">
            <SectionCard title="资产曲线" description="橙色为策略收益曲线，蓝色为买入并持有（全仓不动）基准。">
              <div ref={equityChartContainerRef} className="h-[240px] w-full" />
              <div className="mt-3 flex flex-wrap gap-4 text-xs text-white/55">
                <span className="inline-flex items-center gap-2">
                  <span className="h-2.5 w-2.5 rounded-full bg-[#e67e22]" />策略收益
                </span>
                <span className="inline-flex items-center gap-2">
                  <span className="h-2.5 w-2.5 rounded-full bg-[#38bdf8]" />买入并持有
                </span>
              </div>
            </SectionCard>
            <SectionCard title={rsiKey ? 'RSI 指标' : '回撤曲线'} description={rsiKey ? 'RSI 策略下展示区间强弱指标。' : '观察回测期间的净值回撤过程。'}>
              <div ref={auxChartContainerRef} className="h-[240px] w-full" />
            </SectionCard>
          </section>

          <section className="grid gap-6 xl:grid-cols-2">
            <SectionCard title="回测摘要" description="汇总当前回测的核心信息。">
              <div className="grid gap-3">
                <StatRow label="回测标的" value={result.data_summary?.ticker_display || result.data_summary?.ticker || '示例/CSV'} />
                <StatRow label="股票名称" value={result.data_summary?.ticker_name || (result.data_source === 'online' ? '未识别' : '不适用')} />
                <StatRow label="交易日数量" value={`${result.data_summary?.total_records || 0} 天`} />
                <StatRow label="买入并持有收益" value={formatPercent(result.metrics?.buy_and_hold_return_pct)} />
                <StatRow label="超额收益（策略-买入并持有）" value={formatPercent(result.metrics?.excess_return_pct)} />
                <StatRow label="日收益胜率" value={formatPercent(result.metrics?.daily_win_rate_pct)} />
                <StatRow label="波动率" value={formatPercent(result.metrics?.volatility_pct)} />
                <StatRow label="最佳单日" value={formatPercent(result.metrics?.best_day_pct)} />
                <StatRow label="最差单日" value={formatPercent(result.metrics?.worst_day_pct)} />
              </div>

            </SectionCard>

            <SectionCard title="信号统计" description="查看策略发出的买卖信号数量。">
              <div className="grid grid-cols-3 gap-4">
                <SignalCard label="买入" value={result.signal_summary?.buy || 0} color="text-positive" />
                <SignalCard label="卖出" value={result.signal_summary?.sell || 0} color="text-negative" />
                <SignalCard label="持有" value={result.signal_summary?.hold || 0} color="text-white" />
              </div>
            </SectionCard>
          </section>

          <section className="grid gap-6 xl:grid-cols-[0.92fr_1.08fr]">
            <TableCard title="月度收益" description="按月聚合的回测收益表现。">
              <div className="max-h-[420px] overflow-auto rounded-xl border border-border">
                <table className="min-w-full divide-y divide-white/5 text-sm">
                  <thead className="bg-black/30 text-left text-white/50">
                    <tr>
                      <th className="px-4 py-3 font-medium">月份</th>
                      <th className="px-4 py-3 font-medium">月收益率</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-white/5">
                    {(result.analysis?.monthly_returns || []).length === 0 ? (
                      <tr>
                        <td colSpan={2} className="px-4 py-10 text-center text-white/40">
                          暂无月度收益数据
                        </td>
                      </tr>
                    ) : (
                      result.analysis.monthly_returns.map((item) => (
                        <tr key={item.month}>
                          <td className="px-4 py-3 text-white/75">{item.month}</td>
                          <td className={`px-4 py-3 font-medium ${item.return_pct >= 0 ? 'text-positive' : 'text-negative'}`}>
                            {formatPercent(item.return_pct)}
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </TableCard>

            <TableCard title="交易记录" description="展示策略产生的逐笔交易明细。">
              <div className="max-h-[420px] overflow-auto rounded-xl border border-border">
                <table className="min-w-full divide-y divide-white/5 text-sm">
                  <thead className="bg-black/30 text-left text-white/50">
                    <tr>
                      <th className="px-4 py-3 font-medium">日期</th>
                      <th className="px-4 py-3 font-medium">方向</th>
                      <th className="px-4 py-3 font-medium">价格</th>
                      <th className="px-4 py-3 font-medium">数量</th>
                      <th className="px-4 py-3 font-medium">金额</th>
                      <th className="px-4 py-3 font-medium">手续费</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-white/5">
                    {(result.trades || []).length === 0 ? (
                      <tr>
                        <td colSpan={6} className="px-4 py-10 text-center text-white/40">
                          当前回测没有成交记录
                        </td>
                      </tr>
                    ) : (
                      result.trades.map((trade, index) => (
                        <tr key={`${trade.date}-${trade.type}-${index}`}>
                          <td className="px-4 py-3 text-white/75">{trade.date}</td>
                          <td className={`px-4 py-3 font-medium ${trade.type === 'buy' ? 'text-positive' : 'text-negative'}`}>
                            {trade.type === 'buy' ? '买入' : '卖出'}
                          </td>
                          <td className="px-4 py-3 text-white/75">{formatNumber(trade.price, 2)}</td>
                          <td className="px-4 py-3 text-white/75">{formatInteger(trade.shares)}</td>
                          <td className="px-4 py-3 text-white/75">{formatCurrency(trade.amount)}</td>
                          <td className="px-4 py-3 text-white/75">{formatCurrency(trade.fee)}</td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </TableCard>
          </section>
        </>
      )}
    </div>
  );
}

function parseApiResponse(responseText) {
  if (!responseText) return null;

  try {
    return JSON.parse(responseText);
  } catch {
    return responseText;
  }
}

function extractErrorMessage(responseData, fallbackText) {
  if (responseData && typeof responseData === 'object' && !Array.isArray(responseData) && 'detail' in responseData) {
    return formatApiDetail(responseData.detail) || fallbackText || '回测请求失败';
  }

  return formatApiDetail(responseData) || fallbackText || '回测请求失败';
}

function formatApiDetail(detail) {
  if (!detail) return '';
  if (typeof detail === 'string') return detail;

  if (Array.isArray(detail)) {
    return detail.map((item) => formatApiValidationItem(item)).filter(Boolean).join('；');
  }

  if (typeof detail === 'object') {
    if (typeof detail.message === 'string') return detail.message;
    if (typeof detail.detail === 'string') return detail.detail;
  }

  return String(detail);
}

function formatApiValidationItem(item) {
  if (!item || typeof item !== 'object') {
    return typeof item === 'string' ? item : String(item || '');
  }

  const fieldPath = formatErrorFieldPath(item.loc);

  if (item.type === 'greater_than_equal' && item.ctx?.ge !== undefined) {
    return `${fieldPath || '该字段'}不能小于 ${item.ctx.ge}。`;
  }

  if (item.type === 'less_than_equal' && item.ctx?.le !== undefined) {
    return `${fieldPath || '该字段'}不能大于 ${item.ctx.le}。`;
  }

  if (item.type === 'greater_than' && item.ctx?.gt !== undefined) {
    return `${fieldPath || '该字段'}必须大于 ${item.ctx.gt}。`;
  }

  if (item.type === 'less_than' && item.ctx?.lt !== undefined) {
    return `${fieldPath || '该字段'}必须小于 ${item.ctx.lt}。`;
  }

  if (item.msg) {
    return fieldPath ? `${fieldPath}：${item.msg}` : item.msg;
  }

  return fieldPath || '请求参数校验失败';
}

function formatErrorFieldPath(loc) {
  if (!Array.isArray(loc)) return '';

  return loc
    .filter((segment) => segment !== 'body')
    .map((segment) => ERROR_FIELD_LABELS[segment] || String(segment))
    .join(' / ');
}

function buildPayload(form, selectedStrategy) {
  return {
    data_source: form.dataSource,
    ticker: form.dataSource === 'online' ? form.ticker.trim() : null,
    start_date: form.startDate,
    end_date: form.endDate,
    capital: Number(form.capital),
    fee_pct: Number(form.feePct),
    strategy_id: selectedStrategy.id,
    strategy_name: selectedStrategy.name,
    csv_content: form.dataSource === 'csv' ? form.csvContent : null,
    csv_filename: form.dataSource === 'csv' ? form.csvFilename : null,
    sample_config: {
      start_price: Number(form.sampleConfig.start_price),
      drift: Number(form.sampleConfig.drift),
      volatility: Number(form.sampleConfig.volatility),
      seed: Number(form.sampleConfig.seed),
    },
  };
}

function buildChartOptions(width, height, ColorType) {
  return {
    width: width || 400,
    height,
    layout: {
      background: { type: ColorType.Solid, color: 'transparent' },
      textColor: 'rgba(255,255,255,0.72)',
    },
    grid: {
      vertLines: { color: 'rgba(255,255,255,0.05)' },
      horzLines: { color: 'rgba(255,255,255,0.05)' },
    },
    rightPriceScale: {
      borderColor: 'rgba(255,255,255,0.1)',
    },
    timeScale: {
      borderColor: 'rgba(255,255,255,0.1)',
      timeVisible: true,
      secondsVisible: false,
    },
    crosshair: {
      vertLine: { color: 'rgba(230,126,34,0.35)' },
      horzLine: { color: 'rgba(230,126,34,0.35)' },
    },
  };
}

function formatParamDisplay(value) {
  if (value === null || value === undefined || value === '') return '--';
  if (typeof value === 'number') {
    return Number.isInteger(value) ? String(value) : String(Number(value).toFixed(4)).replace(/0+$/, '').replace(/\.$/, '');
  }
  if (typeof value === 'boolean') return value ? '是' : '否';
  return String(value);
}

function formatCurrency(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--';
  return new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY', maximumFractionDigits: 2 }).format(Number(value));
}

function formatPercent(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--';
  return `${Number(value).toFixed(2)}%`;
}

function formatNumber(value, digits = 2) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--';
  return Number(value).toFixed(digits);
}

function formatInteger(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '--';
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(Number(value));
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
  return <input {...props} className="w-full rounded-xl border border-border bg-black px-4 py-3 text-sm text-white outline-none transition placeholder:text-white/25 focus:border-primary" />;
}

function SectionCard({ title, description, children }) {
  return (
    <section className="rounded-2xl border border-border bg-card p-6">
      <div className="mb-5">
        <h2 className="text-lg font-semibold text-white">{title}</h2>
        {description && <p className="mt-2 text-sm leading-6 text-white/55">{description}</p>}
      </div>
      {children}
    </section>
  );
}

function TableCard({ title, description, children }) {
  return (
    <section className="rounded-2xl border border-border bg-card p-6">
      <div className="mb-5">
        <h2 className="text-lg font-semibold text-white">{title}</h2>
        {description && <p className="mt-2 text-sm leading-6 text-white/55">{description}</p>}
      </div>
      {children}
    </section>
  );
}

function SummaryPill({ label, value }) {
  return (
    <div className="rounded-2xl border border-border bg-card px-5 py-4">
      <div className="text-xs uppercase tracking-[0.18em] text-white/35">{label}</div>
      <div className="mt-2 text-sm font-medium text-white/80">{value || '--'}</div>
    </div>
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

function MetricCard({ title, value, type }) {
  const numeric = Number(value);
  const isNegative = !Number.isNaN(numeric) && numeric < 0;
  const colorClass = isNegative ? 'text-negative' : 'text-white';

  let displayValue = '--';
  if (type === 'currency') displayValue = formatCurrency(value);
  if (type === 'percent') displayValue = formatPercent(value);
  if (type === 'integer') displayValue = formatInteger(value);
  if (type === 'number') displayValue = formatNumber(value, 3);

  return (
    <div className="rounded-2xl border border-border bg-card p-5">
      <div className="text-sm text-white/45">{title}</div>
      <div className={`mt-3 text-2xl font-semibold ${colorClass}`}>{displayValue}</div>
    </div>
  );
}

function StatRow({ label, value }) {
  return (
    <div className="flex items-center justify-between rounded-xl border border-white/5 bg-black/20 px-4 py-3 text-sm">
      <span className="text-white/50">{label}</span>
      <span className="font-medium text-white/80">{value || '--'}</span>
    </div>
  );
}

function SignalCard({ label, value, color }) {
  return (
    <div className="rounded-2xl border border-border bg-black/20 px-4 py-5 text-center">
      <div className="text-sm text-white/45">{label}</div>
      <div className={`mt-3 text-3xl font-semibold ${color}`}>{formatInteger(value)}</div>
    </div>
  );
}

function HistoryRunCard({ run, isActive, onSelect, onDelete }) {
  const metrics = run.metrics_summary || {};
  const totalReturn = metrics.total_return_pct;
  const maxDrawdown = metrics.max_drawdown_pct;
  const sharpe = metrics.sharpe_ratio;
  const trades = metrics.total_trades;

  const returnColor = totalReturn != null && totalReturn >= 0 ? 'text-positive' : 'text-negative';

  const createdAt = run.created_at ? formatRelativeTime(run.created_at) : '';

  return (
    <div
      className={`group relative cursor-pointer rounded-2xl border p-4 transition ${
        isActive
          ? 'border-primary bg-primary/10 shadow-[0_0_0_1px_rgba(230,126,34,0.25)]'
          : 'border-border bg-black/20 hover:border-white/20'
      }`}
      onClick={onSelect}
    >
      <button
        type="button"
        onClick={(event) => {
          event.stopPropagation();
          onDelete();
        }}
        className="absolute right-3 top-3 hidden rounded-full p-1 text-white/25 transition hover:bg-white/10 hover:text-red-400 group-hover:block"
        title="删除"
      >
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <line x1="18" y1="6" x2="6" y2="18" />
          <line x1="6" y1="6" x2="18" y2="18" />
        </svg>
      </button>

      <div className="mb-2 pr-6 text-sm font-medium text-white leading-tight truncate" title={run.title}>
        {run.title || '回测记录'}
      </div>

      <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-white/55">
        {totalReturn != null && (
          <span>
            收益{' '}
            <span className={`font-medium ${returnColor}`}>
              {totalReturn >= 0 ? '+' : ''}{Number(totalReturn).toFixed(2)}%
            </span>
          </span>
        )}
        {maxDrawdown != null && (
          <span>
            回撤{' '}
            <span className="font-medium text-white/70">
              -{Math.abs(Number(maxDrawdown)).toFixed(2)}%
            </span>
          </span>
        )}
        {sharpe != null && (
          <span>
            夏普{' '}
            <span className="font-medium text-white/70">{Number(sharpe).toFixed(2)}</span>
          </span>
        )}
        {trades != null && (
          <span>
            交易{' '}
            <span className="font-medium text-white/70">{trades}笔</span>
          </span>
        )}
      </div>

      <div className="mt-2 flex items-center gap-3 text-[11px] text-white/30">
        <span>{run.start_date} ~ {run.end_date}</span>
        {createdAt && <span>{createdAt}</span>}
        {run.status === 'failed' && (
          <span className="rounded-full bg-red-500/20 px-2 py-0.5 text-[10px] text-red-300">失败</span>
        )}
      </div>
    </div>
  );
}

function formatRelativeTime(isoString) {
  try {
    const date = new Date(isoString);
    const now = new Date();
    const diffMs = now - date;
    const diffMinutes = Math.floor(diffMs / 60000);
    if (diffMinutes < 1) return '刚刚';
    if (diffMinutes < 60) return `${diffMinutes}分钟前`;
    const diffHours = Math.floor(diffMinutes / 60);
    if (diffHours < 24) return `${diffHours}小时前`;
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 30) return `${diffDays}天前`;
    return date.toLocaleDateString('zh-CN');
  } catch {
    return '';
  }
}
