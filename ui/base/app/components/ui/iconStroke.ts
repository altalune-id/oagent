// iconStroke draws every Lucide icon at the design's 1.75 stroke instead of Lucide's default 2.
export function iconStroke(content: string): string {
  return content.replaceAll('stroke-width="2"', 'stroke-width="1.75"');
}
