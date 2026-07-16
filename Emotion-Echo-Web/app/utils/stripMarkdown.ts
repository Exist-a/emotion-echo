export interface StripMarkdownOptions {
  removeCodeBlocks?: boolean
  removeInlineCode?: boolean
  removeUrls?: boolean
  removeMarkdownSyntax?: boolean
  collapseWhitespace?: boolean
}

const defaultOptions: StripMarkdownOptions = {
  removeCodeBlocks: true,
  removeInlineCode: true,
  removeUrls: true,
  removeMarkdownSyntax: true,
  collapseWhitespace: true,
}

export function stripMarkdown(text: string, options?: StripMarkdownOptions): string {
  const opts = { ...defaultOptions, ...options }

  if (!text) return ''

  let result = text

  if (opts.removeCodeBlocks) {
    result = result.replace(/```[\s\S]*?```/g, '')
    result = result.replace(/```[\s\S]*?$/gm, '')
  }

  if (opts.removeInlineCode) {
    result = result.replace(/`[^`]+`/g, '')
  }

  if (opts.removeUrls) {
    result = result.replace(/https?:\/\/[^\s\u4e00-\u9fa5]+/gi, '')
    result = result.replace(/\[([^\]]+)\]\([^\)]+\)/g, '$1')
  }

  if (opts.removeMarkdownSyntax) {
    result = result.replace(/^#+\s*/gm, '')
    result = result.replace(/^\s*[-*+]\s+/gm, '')
    result = result.replace(/^\s*\d+\.\s+/gm, '')
    result = result.replace(/^\s*>\s+/gm, '')
    result = result.replace(/\*\*([^*]+)\*\*/g, '$1')
    result = result.replace(/\*([^*]+)\*/g, '$1')
    result = result.replace(/__([^_]+)__/g, '$1')
    result = result.replace(/_([^_]+)_/g, '$1')
    result = result.replace(/~~([^~]+)~~/g, '$1')
    result = result.replace(/\|([^|]+)\|/g, '$1')
    result = result.replace(/!\[([^\]]*)\]\([^)]+\)/g, '$1')
    result = result.replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
  }

  if (opts.collapseWhitespace) {
    result = result.replace(/\n{3,}/g, '\n\n')
    result = result.replace(/[ \t]+/g, ' ')
    result = result.trim()
  }

  return result
}

export function isMostlyCode(text: string): boolean {
  const codePatterns = [
    /```/,
    /`[^`]+`/,
    /\b(function|const|let|var|if|else|for|while|return|import|export|class|def|async|await)\b/,
    /[{}\[\]();]/,
  ]

  let matchCount = 0
  for (const pattern of codePatterns) {
    if (pattern.test(text)) {
      matchCount++
    }
  }

  return matchCount >= 2
}

export function extractReadableText(text: string): string {
  const stripped = stripMarkdown(text)

  if (isMostlyCode(stripped)) {
    return ''
  }

  return stripped
}