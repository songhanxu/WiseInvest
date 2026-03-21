const { request, streamRequest } = require('./api');

function createConversation(agentType, title = '') {
  return request({
    url: '/api/v1/conversations',
    method: 'POST',
    needAuth: true,
    data: {
      agent_type: agentType,
      title,
    },
  });
}

function sendStreamMessage(conversationId, content, handlers = {}) {
  return streamRequest({
    url: '/api/v1/messages/stream',
    data: {
      conversation_id: conversationId,
      content,
    },
    onChunk: handlers.onChunk,
    onDone: handlers.onDone,
    onError: handlers.onError,
  });
}

module.exports = {
  createConversation,
  sendStreamMessage,
};
