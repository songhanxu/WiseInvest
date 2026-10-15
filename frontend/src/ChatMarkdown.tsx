import Markdown from "react-markdown";
import type { Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkBreaks from "remark-breaks";
import remarkCjkFriendly from "remark-cjk-friendly";

const plugins = [remarkGfm, remarkBreaks, remarkCjkFriendly];
const components: Components = {
  a: ({ node: _node, ...props }) => <a {...props} target="_blank" rel="noopener noreferrer" />,
  table: ({ node: _node, ...props }) => (
    <div className="chat-table-scroll" role="region" aria-label="分析数据表" tabIndex={0}>
      <table {...props} />
    </div>
  ),
  img: ({ node: _node, ...props }) => <img {...props} loading="lazy" />
};

export function ChatMarkdown({ content }: { content: string }) {
  return (
    <div className="chat-markdown">
      <Markdown remarkPlugins={plugins} components={components}>{content}</Markdown>
    </div>
  );
}
