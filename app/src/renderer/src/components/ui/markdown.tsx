/**
 * Markdown — renders AI-generated text (agent summaries) as GitHub-flavored
 * markdown instead of plain text.
 *
 * Wraps react-markdown + remark-gfm with prose typography tuned to the app's
 * color tokens, so inline code, lists, bold, etc. match the surrounding UI.
 */

import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { cn } from '@/lib/utils'

interface MarkdownProps {
  children: string
  className?: string
}

export function Markdown({ children, className }: MarkdownProps): JSX.Element {
  return (
    <div
      className={cn(
        // prose-sm keeps the rhythm tight inside cards / detail panels.
        'prose prose-sm dark:prose-invert max-w-none',
        // loosen prose's own coloring so we follow theme tokens
        'prose-p:text-foreground prose-p:my-1.5 prose-p:leading-relaxed',
        'prose-strong:text-foreground',
        'prose-ul:my-1.5 prose-ol:my-1.5 prose-li:my-0',
        'prose-headings:text-foreground prose-headings:font-semibold',
        'prose-a:text-primary prose-a:no-underline hover:prose-a:underline',
        className
      )}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          // inline code: subtle chip; fenced blocks: scrollable panel
          code({ inline, className: codeClass, children, ...props }) {
            if (inline) {
              return (
                <code
                  className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em] text-foreground"
                  {...props}
                >
                  {children}
                </code>
              )
            }
            return (
              <code className={cn('block overflow-x-auto rounded-md bg-muted p-2 font-mono text-xs', codeClass)} {...props}>
                {children}
              </code>
            )
          },
          // neutral blockquote, not prose's default gray-on-gray
          blockquote({ children, ...props }) {
            return (
              <blockquote
                className="my-1.5 border-l-2 border-primary/40 pl-3 italic text-muted-foreground"
                {...props}
              >
                {children}
              </blockquote>
            )
          }
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  )
}
