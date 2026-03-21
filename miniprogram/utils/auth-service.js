const { request } = require('./api');

function wechatLogin(code) {
  return request({
    url: '/api/v1/auth/wechat/login',
    method: 'POST',
    data: { code },
  });
}

function sendPhoneCode(phone) {
  return request({
    url: '/api/v1/auth/phone/send-code',
    method: 'POST',
    data: { phone },
    needAuth: true,
  });
}

function bindPhone(phone, code) {
  return request({
    url: '/api/v1/auth/phone/bind',
    method: 'POST',
    data: { phone, code },
    needAuth: true,
  });
}

module.exports = {
  wechatLogin,
  sendPhoneCode,
  bindPhone,
};
