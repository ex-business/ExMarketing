export function renderFreeElementText(text: string) {
  return text.split('\\n').map((line, index) => <span key={`${index}:${line}`}>{line}</span>);
}
