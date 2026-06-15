import CodeMirror from '@uiw/react-codemirror';
import { autocompletion, startCompletion, type CompletionContext } from '@codemirror/autocomplete';
import { StreamLanguage } from '@codemirror/language';
import { lua } from '@codemirror/legacy-modes/mode/lua';
import { EditorView, keymap } from '@codemirror/view';
import { useRef } from 'react';

export type LuaConfigVariable = {
  key: string;
  label: string;
  type: string;
  description?: string;
  options?: string[];
};

type Props = {
  value: string;
  variables: LuaConfigVariable[];
  onChange: (value: string) => void;
  onHint: () => void;
};

function luaConfigCompletions(variables: LuaConfigVariable[]) {
  const fieldOptions = variables.map((field) => ({
    label: field.key,
    type: 'variable',
    detail: field.label,
    info: field.description || field.type,
  }));
  const apiOptions = [
    { label: 'Context.get', type: 'function', detail: '读取上下文', info: 'Context.get("event.subject.id")' },
    { label: 'Resource.default.get', type: 'function', detail: '读取默认资源', info: 'Resource.default.get("key")' },
    { label: 'ResourceRef', type: 'function', detail: '显式资源引用', info: 'ResourceRef("resource_code").get("key")' },
    { label: 'Node.get', type: 'function', detail: '读取上游输出', info: 'Node.get("normalize", "normalized_event", "$.attributes.channel")' },
    { label: 'Template.render', type: 'function', detail: '渲染模板', info: 'Template.render("checkin:${subject.id}:${date}")' },
    { label: 'Time.date', type: 'function', detail: '格式化日期', info: 'Time.date("Asia/Shanghai")' },
  ];
  return (context: CompletionContext) => {
    const line = context.state.doc.lineAt(context.pos);
    const before = line.text.slice(0, context.pos - line.from);
    const valueMatch = before.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:\{[^}\n]*)?(["']?)([A-Za-z0-9_-]*)$/);
    if (valueMatch) {
      const [, fieldKey, quote, prefix] = valueMatch;
      const field = variables.find((item) => item.key === fieldKey);
      if (field?.options?.length) {
        const from = context.pos - prefix.length;
        return {
          from,
          options: field.options.map((option) => ({
            label: option,
            type: 'constant',
            detail: `${field.label} 选项`,
            info: field.description || field.type,
            apply: quote ? `${option}${quote}` : JSON.stringify(option),
          })),
          validFor: /^[A-Za-z0-9_-]*$/,
        };
      }
    }
    const word = context.matchBefore(/[A-Za-z_][A-Za-z0-9_.]*/);
    if (!word && !context.explicit) return null;
    return {
      from: word?.from ?? context.pos,
      options: [...fieldOptions, ...apiOptions],
      validFor: /^[A-Za-z_][A-Za-z0-9_.]*$/,
    };
  };
}

export function LuaConfigCodeEditor({ value, variables, onChange, onHint }: Props) {
  const editorViewRef = useRef<EditorView | null>(null);
  const openCompletion = () => {
    onHint();
    window.requestAnimationFrame(() => {
      if (editorViewRef.current) startCompletion(editorViewRef.current);
    });
  };
  return (
    <div
      className="node-lua-code-shell"
      onFocus={onHint}
      onKeyDown={(event) => {
        if (event.key === ' ') onHint();
      }}
      onKeyUp={(event) => {
        if (event.key === ' ') openCompletion();
      }}
    >
      <CodeMirror
        value={value}
        height="180px"
        onCreateEditor={(view) => {
          editorViewRef.current = view;
        }}
        basicSetup={{ lineNumbers: true, foldGutter: true, highlightActiveLine: true }}
        extensions={[
          StreamLanguage.define(lua),
          autocompletion({
            override: [luaConfigCompletions(variables)],
            activateOnTyping: true,
            activateOnTypingDelay: 0,
            interactionDelay: 0,
          }),
          keymap.of([{
            key: 'Space',
            run: (view) => {
              view.dispatch(view.state.replaceSelection(' '));
              window.requestAnimationFrame(() => startCompletion(view));
              openCompletion();
              return true;
            },
          }]),
        ]}
        placeholder="scope = 'USER'\nmaxCount = Resource.default.get('daily_checkin_max_count')\nconsumeMode = 'ATOMIC'"
        onChange={onChange}
      />
    </div>
  );
}
