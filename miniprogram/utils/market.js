const MARKETS = [
  {
    id: 'a_share',
    displayName: 'A 股',
    subtitle: '沪深北交所',
    description: '个股分析 · 行业研究 · 政策解读',
    icon: '📈',
    gradientStart: '#B71C1C',
    gradientEnd: '#E53935',
    welcomeMessage: '你好！我是你的A股投资分析助手。\n\n我可以帮你分析个股、行业趋势、技术形态、基本面数据以及政策影响。\n\n有什么想聊的？',
  },
  {
    id: 'us_stock',
    displayName: '美 股',
    subtitle: 'NYSE · NASDAQ',
    description: '财报分析 · 成长股 · 宏观经济',
    icon: '💵',
    gradientStart: '#0D47A1',
    gradientEnd: '#1976D2',
    welcomeMessage: 'Hi！我是你的美股投资分析助手。\n\n我可以帮你研究美股个股、解读财报、分析宏观经济数据以及美联储政策影响。\n\n想聊哪只股票？',
  },
  {
    id: 'crypto',
    displayName: '币 圈',
    subtitle: '加密货币',
    description: '现货合约 · 链上数据 · DeFi',
    icon: '₿',
    gradientStart: '#E65100',
    gradientEnd: '#FF9800',
    welcomeMessage: '你好！我是你的加密货币分析助手。\n\n我可以分析BTC/ETH走势、合约机会、链上数据、DeFi协议以及加密市场结构。\n\n有什么想分析的？',
  },
];

function getMarketById(id) {
  return MARKETS.find((item) => item.id === id) || MARKETS[0];
}

module.exports = {
  MARKETS,
  getMarketById,
};
