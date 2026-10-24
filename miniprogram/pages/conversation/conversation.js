const { getMarketById } = require('../../utils/market');
const { loadConversations, upsertConversation, removeConversationById } = require('../../utils/storage');
const { createConversation, sendStreamMessage } = require('../../utils/conversation-service');

function createId() {
  return `${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

function nowISO() {
  return new Date().toISOString();
}

function generateTitle(firstUserMessage) {
  let msg = (firstUserMessage || '').trim();
  const fillers = ['请帮我', '帮我', '请你帮我', '请你', '请问', '你能帮我', '能帮我', '我想要了解', '我想了解', '我想知道', '我需要', '我要'];
  fillers.forEach((f) => {
    msg = msg.replace(f, '');
  });
  msg = msg.replace(/一下/g, '').replace(/\s+/g, '').trim();

  const actionVerbs = ['分析', '查询', '查看', '对比', '预测', '解读', '介绍', '解析', '研究', '评估', '总结'];
  const hasVerb = actionVerbs.some((v) => msg.startsWith(v));
  if (!hasVerb) {
    const endings = ['怎么样', '如何', '怎么了', '怎么看', '好不好', '吗', '？', '?'];
    for (let i = 0; i < endings.length; i += 1) {
      const e = endings[i];
      if (msg.endsWith(e)) {
        msg = msg.slice(0, msg.length - e.length);
        break;
      }
    }
    msg = `分析${msg}`;
  }

  msg = msg.slice(0, 13).replace(/[，。！？,.!?、]+$/g, '');
  return msg || '新对话';
}

Page({
  data: {
    market: getMarketById('a_share'),
    conversationId: '',
    backendConversationId: 0,
    messages: [],
    inputText: '',
    isLoading: false,
    errorMessage: '',
    scrollIntoView: 'msg-bottom',
    lastRegeneratableAssistantId: '',
    createdAt: '',
    keyboardHeight: 0,
    windowHeight: 812,
  },

  streamTask: null,
  currentStreamingId: '',

  async onLoad(options) {
    const windowInfo = wx.getWindowInfo ? wx.getWindowInfo() : { statusBarHeight: 20, windowHeight: 812 };
    const windowHeight = windowInfo.windowHeight || 812;
    const marketId = options.market || 'a_share';
    const conversationId = options.conversationId || '';
    const market = getMarketById(marketId);

    this.setData({ windowHeight, market, conversationId });

    if (conversationId) {
      const found = loadConversations().find((item) => item.id === conversationId);
      if (found) {
        this.setData({
          messages: found.messages || [],
          conversationId: found.id,
          backendConversationId: found.backendConversationId || 0,
          createdAt: found.createdAt || nowISO(),
        }, this.updateRegenerateAnchor);
        this.scrollToBottom();
      }
    }

    if (!this.data.messages.length) {
      const welcome = {
        id: createId(),
        role: 'assistant',
        content: market.welcomeMessage,
        timestamp: nowISO(),
        isStreaming: false,
        thinkingLines: [],
      };
      this.setData({ messages: [welcome], createdAt: nowISO() }, this.updateRegenerateAnchor);
    }
  },

  onShow() {
    this.setData({ keyboardHeight: 0 });
  },

  onHide() {
    this.setData({ keyboardHeight: 0 });
  },

  onUnload() {
    if (this.streamTask && this.streamTask.abort) {
      this.streamTask.abort();
    }
  },

  async ensureBackendConversation() {
    if (this.data.backendConversationId) return this.data.backendConversationId;
    try {
      const resp = await createConversation(this.data.market.id, `${this.data.market.displayName} Conversation`);
      const backendId = Number(resp.id || 0);
      this.setData({ backendConversationId: backendId });
      return backendId;
    } catch (error) {
      this.setData({ errorMessage: error.message || '会话初始化失败' });
      return 0;
    }
  },

  onResize(e) {
    const windowHeight = Number(e.size && e.size.windowHeight);
    if (windowHeight > 0) this.setData({ windowHeight });
  },

  onInput(e) {
    this.setData({ inputText: e.detail.value || '' });
  },

  onKeyboardHeightChange(e) {
    const reportedHeight = Number(e && e.detail && e.detail.height);
    const rawHeight = Number.isFinite(reportedHeight) ? Math.max(0, reportedHeight) : 0;
    const keyboardHeight = Math.min(rawHeight, this.data.windowHeight || 812);
    this.setData({
      keyboardHeight,
    }, () => {
      if (keyboardHeight > 0) {
        this.scrollToBottom();
      }
    });
  },

  onInputBlur() {
    this.setData({ keyboardHeight: 0 });
  },

  async sendMessage() {
    const content = (this.data.inputText || '').trim();
    if (!content || this.data.isLoading) return;

    this.setData({ isLoading: true, errorMessage: '' });
    const backendConversationId = await this.ensureBackendConversation();
    if (!backendConversationId) { this.setData({ isLoading: false }); return; }

    const userMessage = {
      id: createId(),
      role: 'user',
      content,
      timestamp: nowISO(),
      isStreaming: false,
      thinkingLines: [],
    };

    const streamingId = createId();
    const assistantMessage = {
      id: streamingId,
      role: 'assistant',
      content: '',
      timestamp: nowISO(),
      isStreaming: true,
      thinkingLines: [],
    };

    const messages = [...this.data.messages, userMessage, assistantMessage];
    this.currentStreamingId = streamingId;

    this.setData({
      messages,
      inputText: '',
      isLoading: true,
      errorMessage: '',
    }, () => {
      this.scrollToBottom();
      this.persistConversation(messages);
      this.updateRegenerateAnchor();
    });

    this.streamTask = sendStreamMessage(backendConversationId, content, {
      onChunk: (chunk) => this.handleStreamChunk(streamingId, chunk),
      onDone: () => this.finishStreaming(streamingId),
      onError: (error) => this.failStreaming(streamingId, error),
    });
  },

  handleStreamChunk(streamingId, chunk) {
    const messages = [...this.data.messages];
    const idx = messages.findIndex((m) => m.id === streamingId);
    if (idx < 0) return;

    const target = { ...messages[idx] };
    if (chunk.type === 'thought') {
      const lines = (chunk.content || '')
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean);

      const thinkingLines = [...(target.thinkingLines || [])];
      lines.forEach((line) => {
        if (thinkingLines.length >= 4) return;
        if (thinkingLines[thinkingLines.length - 1] !== line) {
          thinkingLines.push(line);
        }
      });
      target.thinkingLines = thinkingLines;
    } else {
      target.content = `${target.content || ''}${chunk.content || ''}`;
    }

    messages[idx] = target;
    this.setData({ messages }, () => this.scrollToBottom());
  },

  finishStreaming(streamingId) {
    if (this.currentStreamingId !== streamingId) return;

    const messages = [...this.data.messages];
    const idx = messages.findIndex((m) => m.id === streamingId);
    if (idx >= 0) {
      messages[idx] = { ...messages[idx], isStreaming: false };
    }

    this.setData({ isLoading: false, messages }, () => {
      this.persistConversation(messages);
      this.updateRegenerateAnchor();
      this.scrollToBottom();
    });

    this.currentStreamingId = '';
  },

  failStreaming(streamingId, error) {
    const messages = [...this.data.messages].filter((m) => m.id !== streamingId);
    this.setData({
      messages,
      isLoading: false,
      errorMessage: error.message || '消息发送失败',
    }, () => {
      this.persistConversation(messages);
      this.updateRegenerateAnchor();
    });

    this.currentStreamingId = '';
  },

  async onRegenerate() {
    if (this.data.isLoading) return;

    const messages = [...this.data.messages];
    const lastUser = [...messages].reverse().find((m) => m.role === 'user');
    if (!lastUser) return;

    for (let i = messages.length - 1; i >= 0; i -= 1) {
      if (messages[i].role === 'assistant') {
        messages.splice(i, 1);
        break;
      }
    }

    this.setData({ isLoading: true, errorMessage: '' });
    const backendConversationId = await this.ensureBackendConversation();
    if (!backendConversationId) { this.setData({ isLoading: false }); return; }
    const streamingId = createId();
    messages.push({ id: streamingId, role: 'assistant', content: '', timestamp: nowISO(), isStreaming: true, thinkingLines: [] });
    this.currentStreamingId = streamingId;
    this.setData({ messages, isLoading: true, errorMessage: '' }, () => {
      this.persistConversation(messages);
      this.updateRegenerateAnchor();
      this.scrollToBottom();
    });
    this.streamTask = sendStreamMessage(backendConversationId, lastUser.content, {
      onChunk: (chunk) => this.handleStreamChunk(streamingId, chunk),
      onDone: () => this.finishStreaming(streamingId),
      onError: (error) => this.failStreaming(streamingId, error),
    });
  },

  openMenu() {
    if (this.data.isLoading) return;
    wx.showActionSheet({
      itemList: ['返回首页', '清空对话'],
      success: ({ tapIndex }) => {
        if (tapIndex === 0) {
          wx.reLaunch({ url: '/pages/home/home' });
          return;
        }
        if (tapIndex === 1) {
          const welcome = {
            id: createId(),
            role: 'assistant',
            content: this.data.market.welcomeMessage,
            timestamp: nowISO(),
            isStreaming: false,
            thinkingLines: [],
          };
          if (this.data.conversationId) removeConversationById(this.data.conversationId);
          this.setData({ messages: [welcome], conversationId: '', backendConversationId: 0, createdAt: nowISO(), errorMessage: '' }, () => {
            this.updateRegenerateAnchor();
            this.scrollToBottom();
          });
        }
      },
    });
  },

  goBack() {
    if (getCurrentPages().length > 1) {
      wx.navigateBack();
      return;
    }
    wx.reLaunch({ url: '/pages/home/home' });
  },

  updateRegenerateAnchor() {
    const assistant = [...this.data.messages].reverse().find((m) => m.role === 'assistant' && !m.isStreaming);
    this.setData({ lastRegeneratableAssistantId: assistant ? assistant.id : '' });
  },

  persistConversation(messages) {
    if (!messages.some((m) => m.role === 'user')) return;
    const id = this.data.conversationId || createId();
    const firstUser = messages.find((m) => m.role === 'user');
    const title = firstUser ? generateTitle(firstUser.content) : '';

    const payload = {
      id,
      backendConversationId: this.data.backendConversationId,
      agentType: this.data.market.id,
      title: title || `${this.data.market.displayName} · 新对话`,
      messages,
      updatedAt: nowISO(),
      createdAt: this.data.createdAt || nowISO(),
    };

    upsertConversation(payload);
    if (!this.data.conversationId) {
      this.setData({ conversationId: id, createdAt: payload.createdAt });
    }
  },

  scrollToBottom() {
    this.setData({ scrollIntoView: '' }, () => {
      this.setData({ scrollIntoView: 'msg-bottom' });
    });
  },
});
