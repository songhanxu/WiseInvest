const { getBaseURL } = require('./config');

function normalizeErrorMessage(errMsg) {
  const raw = errMsg || '网络请求失败';
  if (/url not in domain list/i.test(raw)) {
    const baseURL = getBaseURL();
    return `当前域名未加入小程序 request 合法域名：${baseURL}`;
  }
  return raw;
}

function request({ url, method = 'GET', data, needAuth = false }) {
  const app = getApp();
  const headers = {
    'Content-Type': 'application/json',
  };

  if (needAuth && app?.globalData?.auth?.token) {
    headers.Authorization = `Bearer ${app.globalData.auth.token}`;
  }

  return new Promise((resolve, reject) => {
    wx.request({
      url: `${getBaseURL()}${url}`,
      method,
      data,
      header: headers,
      success: (res) => {
        if (res.statusCode >= 200 && res.statusCode < 300) {
          resolve(res.data);
          return;
        }
        const msg = (res.data && res.data.error) || `HTTP ${res.statusCode}`;
        reject(new Error(msg));
      },
      fail: (err) => reject(new Error(normalizeErrorMessage(err.errMsg))),
    });
  });
}

function decodeChunk(arrayBuffer) {
  try {
    if (typeof TextDecoder !== 'undefined') {
      return new TextDecoder('utf-8').decode(arrayBuffer);
    }
  } catch (_) {}

  const bytes = new Uint8Array(arrayBuffer);
  let result = '';
  for (let i = 0; i < bytes.length; i += 1) {
    result += String.fromCharCode(bytes[i]);
  }
  try {
    return decodeURIComponent(escape(result));
  } catch (_) {
    return result;
  }
}

function streamRequest({ url, data, onChunk, onDone, onError }) {
  const app = getApp();
  const headers = {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
  };

  if (app?.globalData?.auth?.token) {
    headers.Authorization = `Bearer ${app.globalData.auth.token}`;
  }

  let sseBuffer = '';

  const task = wx.request({
    url: `${getBaseURL()}${url}`,
    method: 'POST',
    header: headers,
    data,
    enableChunked: true,
    responseType: 'arraybuffer',
    success: (res) => {
      if (res.statusCode < 200 || res.statusCode >= 300) {
        const msg = res.data?.error || `HTTP ${res.statusCode}`;
        if (onError) onError(new Error(msg));
        return;
      }
      if (onDone) onDone();
    },
    fail: (err) => {
      if (onError) onError(new Error(normalizeErrorMessage(err.errMsg)));
    },
  });

  if (task && task.onChunkReceived) {
    task.onChunkReceived((res) => {
      const text = decodeChunk(res.data || new ArrayBuffer(0));
      sseBuffer += text;

      const lines = sseBuffer.split('\n');
      sseBuffer = lines.pop() || '';

      lines.forEach((line) => {
        const trimmed = (line || '').trim();
        if (!trimmed.startsWith('data: ')) return;

        const payload = trimmed.slice(6);
        if (payload === '[DONE]') {
          return;
        }

        try {
          const json = JSON.parse(payload);
          if (json.error) {
            if (onError) onError(new Error(json.error));
            return;
          }
          if (onChunk) {
            onChunk({
              type: json.type || 'content',
              content: json.content || '',
            });
          }
        } catch (_) {}
      });
    });
  }

  return task;
}

module.exports = {
  request,
  streamRequest,
};
