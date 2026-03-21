const API_CONFIG = {
  // 真机调试必须使用已在微信公众平台配置为“request 合法域名”的 HTTPS 域名
  // 例如：https://api.xxx.com（不可使用 http / localhost / IP）
  baseURL: 'https://nonimbricative-ungoitered-joyce.ngrok-free.dev',
};

function getBaseURL() {
  return API_CONFIG.baseURL.replace(/\/$/, '');
}

module.exports = {
  API_CONFIG,
  getBaseURL,
};
