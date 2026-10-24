const CONVERSATIONS_KEY = 'conversations';

function loadConversations() {
  const stored = wx.getStorageSync(CONVERSATIONS_KEY) || [];
  return stored.filter((item) => Array.isArray(item.messages) && item.messages.some((message) => message.role === 'user'));
}

function saveConversations(list) {
  wx.setStorageSync(CONVERSATIONS_KEY, list || []);
}

function upsertConversation(conversation) {
  const list = loadConversations().filter((item) => item.id !== conversation.id);
  list.unshift(conversation);
  saveConversations(list);
}

function removeConversationById(id) {
  const list = loadConversations().filter((item) => item.id !== id);
  saveConversations(list);
}

module.exports = {
  loadConversations,
  saveConversations,
  upsertConversation,
  removeConversationById,
};
