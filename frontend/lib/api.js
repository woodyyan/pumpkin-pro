export async function readApiResponse(response) {
  const responseText = await response.text();
  if (!responseText) return null;

  const contentType = response.headers.get('content-type') || '';
  if (contentType.includes('application/json')) {
    try {
      return JSON.parse(responseText);
    } catch {
      return responseText;
    }
  }

  try {
    return JSON.parse(responseText);
  } catch {
    return responseText;
  }
}

export async function requestJson(input, init, fallbackMessage = '请求失败') {
  const response = await fetch(input, init);
  const data = await readApiResponse(response);

  if (!response.ok) {
    throw new Error(extractApiErrorMessage(data, fallbackMessage));
  }

  return data;
}

export function extractApiErrorMessage(responseData, fallbackText = '请求失败') {
  if (responseData && typeof responseData === 'object' && !Array.isArray(responseData) && 'detail' in responseData) {
    return formatApiDetail(responseData.detail) || fallbackText;
  }

  return formatApiDetail(responseData) || fallbackText;
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

  const labels = {
    ticker: '股票代码',
    capital: '初始资金',
    fee_pct: '手续费率',
    strategy_params: '策略参数',
  };

  return loc
    .filter((segment) => segment !== 'body')
    .map((segment) => labels[segment] || String(segment))
    .join(' / ');
}
