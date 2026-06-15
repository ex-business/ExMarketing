import { autocompletion, completionKeymap, startCompletion, type Completion, type CompletionContext } from '@codemirror/autocomplete';
import { StreamLanguage } from '@codemirror/language';
import { lua } from '@codemirror/legacy-modes/mode/lua';
import { linter, lintGutter, type Diagnostic } from '@codemirror/lint';
import { EditorView, keymap } from '@codemirror/view';
import CodeMirror from '@uiw/react-codemirror';
import luaparse from 'luaparse';
import { useEffect, useMemo, useRef, useState } from 'react';

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

type LuaSyntaxStatus = {
  ok: boolean;
  message: string;
  line?: number;
  column?: number;
};

const CONTEXT_METHODS: Completion[] = [
  { label: 'get', type: 'method', detail: 'Context', info: 'Context.get("event.subject.id") 读取流程上下文字段。', apply: 'get("")' },
  { label: 'set', type: 'method', detail: 'Context', info: 'Context.set("vars.key", value) 写入流程上下文。', apply: 'set("", )' },
  { label: 'has', type: 'method', detail: 'Context', info: 'Context.has("event.eventId") 判断上下文路径是否存在。', apply: 'has("")' },
  { label: 'event', type: 'property', detail: 'Context', info: '当前标准事件对象。' },
  { label: 'vars', type: 'property', detail: 'Context', info: '流程公共参数。' },
  { label: 'node', type: 'property', detail: 'Context', info: '当前节点运行上下文。' },
];

const RESOURCE_METHODS: Completion[] = [
  { label: 'get', type: 'method', detail: 'Resource', info: 'resource.get("key") 读取资源中的值。', apply: 'get("")' },
  { label: 'set', type: 'method', detail: 'Resource', info: 'resource.set("key", value) 写入资源。', apply: 'set("", )' },
  { label: 'upsert', type: 'method', detail: 'Resource', info: 'resource.upsert("key", value) 插入或更新。', apply: 'upsert("", )' },
  { label: 'append', type: 'method', detail: 'Resource', info: 'resource.append("key", value) 追加数组/日志项。', apply: 'append("", )' },
  { label: 'delete', type: 'method', detail: 'Resource', info: 'resource.delete("key") 删除资源键。', apply: 'delete("")' },
  { label: 'exists', type: 'method', detail: 'Resource', info: 'resource.exists("key") 判断键是否存在。', apply: 'exists("")' },
  { label: 'ttl', type: 'method', detail: 'Resource', info: 'resource.ttl("key") 读取剩余 TTL。', apply: 'ttl("")' },
];

const NODE_METHODS: Completion[] = [
  { label: 'get', type: 'method', detail: 'Node', info: 'Node.get("nodeId", "output", "$.path") 读取节点输出。', apply: 'get("", "", "")' },
  { label: 'output', type: 'method', detail: 'Node', info: 'Node.output("nodeId", "outputKey") 读取上游输出。', apply: 'output("", "")' },
  { label: 'status', type: 'method', detail: 'Node', info: 'Node.status("nodeId") 读取节点运行状态。', apply: 'status("")' },
  { label: 'error', type: 'method', detail: 'Node', info: 'Node.error("nodeId") 读取节点错误。', apply: 'error("")' },
];

const STRING_METHODS: Completion[] = [
  { label: 'byte', type: 'method', detail: 'string', info: 'string.byte(value, i, j) 返回字符编码。', apply: 'byte()' },
  { label: 'char', type: 'method', detail: 'string', info: 'string.char(...) 按编码生成字符串。', apply: 'char()' },
  { label: 'dump', type: 'method', detail: 'string', info: 'string.dump(fn) 转储函数。', apply: 'dump()' },
  { label: 'format', type: 'method', detail: 'string', info: 'string.format(format, ...) 格式化字符串。', apply: 'format("%s", )' },
  { label: 'len', type: 'method', detail: 'string', info: 'string.len(value) 字符串长度。', apply: 'len()' },
  { label: 'lower', type: 'method', detail: 'string', info: 'string.lower(value) 转小写。', apply: 'lower()' },
  { label: 'upper', type: 'method', detail: 'string', info: 'string.upper(value) 转大写。', apply: 'upper()' },
  { label: 'sub', type: 'method', detail: 'string', info: 'string.sub(value, start, end) 截取字符串。', apply: 'sub(1, -1)' },
  { label: 'find', type: 'method', detail: 'string', info: 'string.find(value, pattern) 查找文本。', apply: 'find("")' },
  { label: 'match', type: 'method', detail: 'string', info: 'string.match(value, pattern) 正则匹配。', apply: 'match("")' },
  { label: 'gsub', type: 'method', detail: 'string', info: 'string.gsub(value, pattern, replacement) 替换文本。', apply: 'gsub("", "")' },
  { label: 'gmatch', type: 'method', detail: 'string', info: 'string.gmatch(value, pattern) 迭代匹配。', apply: 'gmatch("")' },
  { label: 'rep', type: 'method', detail: 'string', info: 'string.rep(value, n) 重复字符串。', apply: 'rep()' },
  { label: 'reverse', type: 'method', detail: 'string', info: 'string.reverse(value) 反转字符串。', apply: 'reverse()' },
];

const TABLE_METHODS: Completion[] = [
  { label: 'insert', type: 'method', detail: 'table', info: 'table.insert(array, value) 追加元素。', apply: 'insert()' },
  { label: 'remove', type: 'method', detail: 'table', info: 'table.remove(array, index) 删除元素。', apply: 'remove()' },
  { label: 'concat', type: 'method', detail: 'table', info: 'table.concat(array, sep) 拼接数组。', apply: 'concat(",")' },
  { label: 'sort', type: 'method', detail: 'table', info: 'table.sort(array) 排序数组。', apply: 'sort()' },
  { label: 'pack', type: 'method', detail: 'table', info: 'table.pack(...) 打包参数为 table。', apply: 'pack()' },
  { label: 'unpack', type: 'method', detail: 'table', info: 'table.unpack(array) 展开数组。', apply: 'unpack()' },
  { label: 'move', type: 'method', detail: 'table', info: 'table.move(a1, f, e, t, a2) 移动元素。', apply: 'move(, , , )' },
  { label: 'len', type: 'method', detail: 'array', info: '#array 获取数组长度。', apply: 'len()' },
  { label: 'contains', type: 'method', detail: 'array', info: '判断数组是否包含某个值。', apply: 'contains()' },
];

const MATH_METHODS: Completion[] = [
  { label: 'abs', type: 'method', detail: 'math', info: 'math.abs(x) 绝对值。', apply: 'abs()' },
  { label: 'ceil', type: 'method', detail: 'math', info: 'math.ceil(x) 向上取整。', apply: 'ceil()' },
  { label: 'floor', type: 'method', detail: 'math', info: 'math.floor(x) 向下取整。', apply: 'floor()' },
  { label: 'max', type: 'method', detail: 'math', info: 'math.max(a, b, ...) 最大值。', apply: 'max()' },
  { label: 'min', type: 'method', detail: 'math', info: 'math.min(a, b, ...) 最小值。', apply: 'min()' },
  { label: 'random', type: 'method', detail: 'math', info: 'math.random([m [, n]]) 随机数。', apply: 'random()' },
  { label: 'randomseed', type: 'method', detail: 'math', info: 'math.randomseed(seed) 设置随机种子。', apply: 'randomseed()' },
  { label: 'sqrt', type: 'method', detail: 'math', info: 'math.sqrt(x) 平方根。', apply: 'sqrt()' },
  { label: 'sin', type: 'method', detail: 'math', info: 'math.sin(x) 正弦。', apply: 'sin()' },
  { label: 'cos', type: 'method', detail: 'math', info: 'math.cos(x) 余弦。', apply: 'cos()' },
  { label: 'tan', type: 'method', detail: 'math', info: 'math.tan(x) 正切。', apply: 'tan()' },
  { label: 'asin', type: 'method', detail: 'math', info: 'math.asin(x) 反正弦。', apply: 'asin()' },
  { label: 'acos', type: 'method', detail: 'math', info: 'math.acos(x) 反余弦。', apply: 'acos()' },
  { label: 'atan', type: 'method', detail: 'math', info: 'math.atan(y, x) 反正切。', apply: 'atan()' },
  { label: 'deg', type: 'method', detail: 'math', info: 'math.deg(x) 弧度转角度。', apply: 'deg()' },
  { label: 'rad', type: 'method', detail: 'math', info: 'math.rad(x) 角度转弧度。', apply: 'rad()' },
  { label: 'exp', type: 'method', detail: 'math', info: 'math.exp(x) e 的 x 次方。', apply: 'exp()' },
  { label: 'log', type: 'method', detail: 'math', info: 'math.log(x, base) 对数。', apply: 'log()' },
  { label: 'fmod', type: 'method', detail: 'math', info: 'math.fmod(x, y) 取模。', apply: 'fmod(, )' },
  { label: 'modf', type: 'method', detail: 'math', info: 'math.modf(x) 拆分整数和小数。', apply: 'modf()' },
  { label: 'huge', type: 'constant', detail: 'math', info: '正无穷常量。' },
  { label: 'maxinteger', type: 'constant', detail: 'math', info: '最大整数。' },
  { label: 'mininteger', type: 'constant', detail: 'math', info: '最小整数。' },
  { label: 'pi', type: 'constant', detail: 'math', info: '圆周率常量。' },
];

const OS_METHODS: Completion[] = [
  { label: 'time', type: 'method', detail: 'os', info: 'os.time() 当前时间戳。', apply: 'time()' },
  { label: 'date', type: 'method', detail: 'os', info: 'os.date(format, time) 格式化时间。', apply: 'date("%Y-%m-%d")' },
  { label: 'difftime', type: 'method', detail: 'os', info: 'os.difftime(t2, t1) 时间差。', apply: 'difftime(, )' },
  { label: 'clock', type: 'method', detail: 'os', info: 'os.clock() CPU 时间。', apply: 'clock()' },
];

const COROUTINE_METHODS: Completion[] = [
  { label: 'create', type: 'method', detail: 'coroutine', info: 'coroutine.create(fn) 创建协程。', apply: 'create(function()\n  \nend)' },
  { label: 'resume', type: 'method', detail: 'coroutine', info: 'coroutine.resume(co, ...) 恢复协程。', apply: 'resume()' },
  { label: 'yield', type: 'method', detail: 'coroutine', info: 'coroutine.yield(...) 挂起协程。', apply: 'yield()' },
  { label: 'status', type: 'method', detail: 'coroutine', info: 'coroutine.status(co) 协程状态。', apply: 'status()' },
  { label: 'wrap', type: 'method', detail: 'coroutine', info: 'coroutine.wrap(fn) 包装协程。', apply: 'wrap(function()\n  \nend)' },
  { label: 'running', type: 'method', detail: 'coroutine', info: 'coroutine.running() 当前协程。', apply: 'running()' },
];

const DEBUG_METHODS: Completion[] = [
  { label: 'traceback', type: 'method', detail: 'debug', info: 'debug.traceback(message) 生成调用栈。', apply: 'traceback()' },
  { label: 'getinfo', type: 'method', detail: 'debug', info: 'debug.getinfo(fn) 获取调试信息。', apply: 'getinfo()' },
];

const IO_METHODS: Completion[] = [
  { label: 'read', type: 'method', detail: 'io', info: 'io.read(...) 读取输入。', apply: 'read()' },
  { label: 'write', type: 'method', detail: 'io', info: 'io.write(...) 写输出。', apply: 'write()' },
  { label: 'open', type: 'method', detail: 'io', info: 'io.open(path, mode) 打开文件。', apply: 'open("", "r")' },
  { label: 'close', type: 'method', detail: 'io', info: 'io.close(file) 关闭文件。', apply: 'close()' },
  { label: 'lines', type: 'method', detail: 'io', info: 'io.lines(path) 按行读取。', apply: 'lines("")' },
];

const PACKAGE_METHODS: Completion[] = [
  { label: 'path', type: 'property', detail: 'package', info: 'Lua module 搜索路径。' },
  { label: 'cpath', type: 'property', detail: 'package', info: 'C module 搜索路径。' },
  { label: 'loaded', type: 'property', detail: 'package', info: '已加载模块表。' },
  { label: 'searchpath', type: 'method', detail: 'package', info: 'package.searchpath(name, path) 搜索模块路径。', apply: 'searchpath("", "")' },
];

const UTF8_METHODS: Completion[] = [
  { label: 'char', type: 'method', detail: 'utf8', info: 'utf8.char(...) 生成 UTF-8 字符。', apply: 'char()' },
  { label: 'codes', type: 'method', detail: 'utf8', info: 'utf8.codes(s) 遍历 UTF-8 编码。', apply: 'codes()' },
  { label: 'codepoint', type: 'method', detail: 'utf8', info: 'utf8.codepoint(s, i, j) 获取码点。', apply: 'codepoint()' },
  { label: 'len', type: 'method', detail: 'utf8', info: 'utf8.len(s) 字符数量。', apply: 'len()' },
  { label: 'offset', type: 'method', detail: 'utf8', info: 'utf8.offset(s, n, i) 字符偏移。', apply: 'offset(, )' },
];

const LUA_SNIPPETS: Completion[] = [
  { label: 'ifelse', type: 'keyword', detail: 'snippet', info: 'if/else 代码块。', apply: 'if  then\n  \nelse\n  \nend' },
  { label: 'forpairs', type: 'keyword', detail: 'snippet', info: 'pairs 遍历 table。', apply: 'for key, value in pairs() do\n  \nend' },
  { label: 'foripairs', type: 'keyword', detail: 'snippet', info: 'ipairs 遍历数组。', apply: 'for index, value in ipairs() do\n  \nend' },
  { label: 'func', type: 'keyword', detail: 'snippet', info: '函数定义。', apply: 'function name()\n  \nend' },
  { label: 'localfunc', type: 'keyword', detail: 'snippet', info: '局部函数定义。', apply: 'local function name()\n  \nend' },
  { label: 'table', type: 'type', detail: 'snippet', info: 'table 字面量。', apply: '{\n  \n}' },
  { label: 'comment', type: 'text', detail: 'snippet', info: '单行注释。', apply: '-- ' },
  { label: 'blockcomment', type: 'text', detail: 'snippet', info: '多行注释。', apply: '--[[\n  \n]]' },
  { label: 'metatable', type: 'keyword', detail: 'snippet', info: 'metatable 模板。', apply: 'setmetatable({}, {\n  __index = function(table, key)\n    return nil\n  end\n})' },
];

const LUA_METAMETHODS: Completion[] = [
  '__index', '__newindex', '__call', '__tostring', '__len', '__pairs', '__ipairs',
  '__add', '__sub', '__mul', '__div', '__mod', '__pow', '__unm', '__concat',
  '__eq', '__lt', '__le', '__gc',
].map((label): Completion => ({ label, type: 'property', detail: 'metamethod', info: `Lua 元方法 ${label}。` }));

const LUA_KEYWORDS: Completion[] = [
  { label: 'local', type: 'keyword', detail: 'Lua', info: '声明局部变量。', apply: 'local ' },
  { label: 'function', type: 'keyword', detail: 'Lua', info: '定义函数。', apply: 'function ' },
  { label: 'return', type: 'keyword', detail: 'Lua', info: '返回值。', apply: 'return ' },
  { label: 'if', type: 'keyword', detail: 'Lua', info: '条件判断。', apply: 'if  then\n  \nend' },
  { label: 'then', type: 'keyword', detail: 'Lua', info: 'if 条件后使用。' },
  { label: 'else', type: 'keyword', detail: 'Lua', info: '条件不成立分支。' },
  { label: 'elseif', type: 'keyword', detail: 'Lua', info: '追加条件分支。', apply: 'elseif  then' },
  { label: 'end', type: 'keyword', detail: 'Lua', info: '结束代码块。' },
  { label: 'for', type: 'keyword', detail: 'Lua', info: 'for 循环。', apply: 'for i = 1,  do\n  \nend' },
  { label: 'in', type: 'keyword', detail: 'Lua', info: '泛型 for 使用。' },
  { label: 'do', type: 'keyword', detail: 'Lua', info: '代码块开始。' },
  { label: 'while', type: 'keyword', detail: 'Lua', info: 'while 循环。', apply: 'while  do\n  \nend' },
  { label: 'repeat', type: 'keyword', detail: 'Lua', info: 'repeat 循环。', apply: 'repeat\n  \nuntil ' },
  { label: 'until', type: 'keyword', detail: 'Lua', info: 'repeat 循环结束条件。' },
  { label: 'break', type: 'keyword', detail: 'Lua', info: '跳出循环。' },
  { label: 'and', type: 'keyword', detail: 'Lua', info: '逻辑与。' },
  { label: 'or', type: 'keyword', detail: 'Lua', info: '逻辑或。' },
  { label: 'not', type: 'keyword', detail: 'Lua', info: '逻辑非。' },
  { label: 'nil', type: 'constant', detail: 'Lua', info: '空值。' },
  { label: 'true', type: 'constant', detail: 'Lua', info: '布尔真。' },
  { label: 'false', type: 'constant', detail: 'Lua', info: '布尔假。' },
];

const LUA_GLOBALS: Completion[] = [
  { label: '_G', type: 'variable', detail: 'Lua', info: '全局环境表。' },
  { label: '_VERSION', type: 'constant', detail: 'Lua', info: 'Lua 版本字符串。' },
  { label: 'pairs', type: 'function', detail: 'Lua', info: 'pairs(table) 遍历 table。', apply: 'pairs()' },
  { label: 'ipairs', type: 'function', detail: 'Lua', info: 'ipairs(array) 顺序遍历数组。', apply: 'ipairs()' },
  { label: 'next', type: 'function', detail: 'Lua', info: 'next(table, key) 获取下一个键。', apply: 'next()' },
  { label: 'type', type: 'function', detail: 'Lua', info: 'type(value) 获取类型。', apply: 'type()' },
  { label: 'tonumber', type: 'function', detail: 'Lua', info: 'tonumber(value) 转数字。', apply: 'tonumber()' },
  { label: 'tostring', type: 'function', detail: 'Lua', info: 'tostring(value) 转字符串。', apply: 'tostring()' },
  { label: 'select', type: 'function', detail: 'Lua', info: 'select(index, ...) 选择可变参数。', apply: 'select()' },
  { label: 'pcall', type: 'function', detail: 'Lua', info: 'pcall(fn, ...) 安全调用函数。', apply: 'pcall()' },
  { label: 'xpcall', type: 'function', detail: 'Lua', info: 'xpcall(fn, msgh, ...) 带错误处理的安全调用。', apply: 'xpcall(, )' },
  { label: 'error', type: 'function', detail: 'Lua', info: 'error(message) 抛出错误。', apply: 'error("")' },
  { label: 'assert', type: 'function', detail: 'Lua', info: 'assert(condition, message) 断言。', apply: 'assert()' },
  { label: 'print', type: 'function', detail: 'Lua', info: 'print(...) 输出调试信息。', apply: 'print()' },
  { label: 'rawequal', type: 'function', detail: 'Lua', info: 'rawequal(a, b) 原始相等比较。', apply: 'rawequal(, )' },
  { label: 'rawget', type: 'function', detail: 'Lua', info: 'rawget(table, key) 原始读取。', apply: 'rawget(, )' },
  { label: 'rawset', type: 'function', detail: 'Lua', info: 'rawset(table, key, value) 原始写入。', apply: 'rawset(, , )' },
  { label: 'getmetatable', type: 'function', detail: 'Lua', info: 'getmetatable(value) 获取 metatable。', apply: 'getmetatable()' },
  { label: 'setmetatable', type: 'function', detail: 'Lua', info: 'setmetatable(table, metatable) 设置 metatable。', apply: 'setmetatable(, )' },
  { label: 'collectgarbage', type: 'function', detail: 'Lua', info: 'collectgarbage(opt) 垃圾回收控制。', apply: 'collectgarbage()' },
  { label: 'require', type: 'function', detail: 'Lua', info: 'require(module) 加载模块。', apply: 'require("")' },
  { label: 'load', type: 'function', detail: 'Lua', info: 'load(chunk) 加载代码块。', apply: 'load()' },
  { label: 'dofile', type: 'function', detail: 'Lua', info: 'dofile(path) 执行文件。', apply: 'dofile("")' },
  { label: 'loadfile', type: 'function', detail: 'Lua', info: 'loadfile(path) 加载文件。', apply: 'loadfile("")' },
  { label: 'string', type: 'namespace', detail: 'Lua', info: 'Lua 字符串标准库。' },
  { label: 'table', type: 'namespace', detail: 'Lua', info: 'Lua table 标准库。' },
  { label: 'math', type: 'namespace', detail: 'Lua', info: 'Lua math 标准库。' },
  { label: 'os', type: 'namespace', detail: 'Lua', info: 'Lua os 时间函数。' },
  { label: 'coroutine', type: 'namespace', detail: 'Lua', info: 'Lua coroutine 标准库。' },
  { label: 'debug', type: 'namespace', detail: 'Lua', info: 'Lua debug 标准库。' },
  { label: 'io', type: 'namespace', detail: 'Lua', info: 'Lua io 标准库。' },
  { label: 'package', type: 'namespace', detail: 'Lua', info: 'Lua package 标准库。' },
  { label: 'utf8', type: 'namespace', detail: 'Lua', info: 'Lua UTF-8 标准库。' },
];

function baseCompletions(variables: LuaConfigVariable[]): Completion[] {
  const fields = variables.map((field): Completion => ({
    label: field.key,
    type: 'variable',
    detail: field.label,
    info: field.description || field.type,
  }));
  const apis: Completion[] = [
    { label: 'Context.get', type: 'function', detail: '读取上下文', info: 'Context.get("event.subject.id")' },
    { label: 'Resource.default.get', type: 'function', detail: '读取默认资源', info: 'Resource.default.get("key")' },
    { label: 'ResourceRef', type: 'function', detail: '显式资源引用', info: 'ResourceRef("resource_code").get("key")' },
    { label: 'Node.get', type: 'function', detail: '读取上游输出', info: 'Node.get("normalize", "normalized_event", "$.attributes.channel")' },
    { label: 'Template.render', type: 'function', detail: '渲染模板', info: 'Template.render("checkin:${subject.id}:${date}")' },
    { label: 'Time.date', type: 'function', detail: '格式化日期', info: 'Time.date("Asia/Shanghai")' },
  ];
  return [...fields, ...apis, ...LUA_KEYWORDS, ...LUA_GLOBALS, ...LUA_SNIPPETS, ...LUA_METAMETHODS];
}

function fieldMethodCompletions(field?: LuaConfigVariable): Completion[] {
  if (!field) return [...STRING_METHODS, ...TABLE_METHODS];
  if (field.type.includes('ARRAY')) return TABLE_METHODS;
  if (field.type.includes('OBJECT')) return [...TABLE_METHODS, { label: 'get', type: 'method', detail: 'object', info: 'object.get("path") 读取对象字段。', apply: 'get("")' }];
  if (field.type.includes('STRING') || field.type.includes('ENUM')) return STRING_METHODS;
  return [
    { label: 'toString', type: 'method', detail: field.type, info: '转换为字符串。', apply: 'toString()' },
  ];
}

function memberCompletions(target: string, variables: LuaConfigVariable[]): Completion[] | undefined {
  if (target === 'Context') return CONTEXT_METHODS;
  if (target === 'Node') return NODE_METHODS;
  if (target === 'Resource' || target === 'Resource.default' || target.startsWith('ResourceRef(')) return RESOURCE_METHODS;
  if (target === 'string') return STRING_METHODS;
  if (target === 'table') return TABLE_METHODS;
  if (target === 'math') return MATH_METHODS;
  if (target === 'os') return OS_METHODS;
  if (target === 'coroutine') return COROUTINE_METHODS;
  if (target === 'debug') return DEBUG_METHODS;
  if (target === 'io') return IO_METHODS;
  if (target === 'package') return PACKAGE_METHODS;
  if (target === 'utf8') return UTF8_METHODS;
  const field = variables.find((item) => item.key === target);
  return field ? fieldMethodCompletions(field) : undefined;
}

function luaCompletionSource(variables: LuaConfigVariable[]) {
  return (context: CompletionContext) => {
    const pos = context.pos;
    const line = context.state.doc.lineAt(pos);
    const before = line.text.slice(0, pos - line.from);
    const memberMatch = before.match(/((?:ResourceRef\([^)]*\)|[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?))\.([A-Za-z_]*)$/);
    if (memberMatch) {
      const [, target, prefix] = memberMatch;
      const options = memberCompletions(target, variables);
      if (options?.length) {
        return {
          from: pos - prefix.length,
          options,
          validFor: /^[A-Za-z_][A-Za-z0-9_]*$/,
        };
      }
    }
    const valueMatch = before.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:\{[^}\n]*)?(["']?)([A-Za-z0-9_-]*)$/);
    if (valueMatch) {
      const [, fieldKey, quote, prefix] = valueMatch;
      const field = variables.find((item) => item.key === fieldKey);
      if (field?.options?.length) {
        return {
          from: pos - prefix.length,
          options: field.options.map((option): Completion => ({
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

    if (/\S\s$/.test(before)) {
      return {
        from: pos,
        options: baseCompletions(variables),
        filter: false,
      };
    }

    const word = context.matchBefore(/[A-Za-z_][A-Za-z0-9_.]*/);
    if (!word && !context.explicit) return null;
    return {
      from: word?.from ?? pos,
      options: baseCompletions(variables),
      validFor: /^[A-Za-z_][A-Za-z0-9_.]*$/,
    };
  };
}

function shouldTriggerLuaCompletion(view: EditorView): boolean {
  const pos = view.state.selection.main.head;
  if (pos <= 0) return false;
  const line = view.state.doc.lineAt(pos);
  const before = line.text.slice(0, pos - line.from);
  if (!before.trim()) return false;
  const lastChar = before.slice(-1);
  if (lastChar === ' ') return /\S\s$/.test(before);
  if (/[A-Za-z0-9_]/.test(lastChar)) return true;
  if (lastChar === '.') {
    return /(?:ResourceRef\([^)]*\)|[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\.$/.test(before);
  }
  return false;
}

function parseLuaSyntax(value: string): LuaSyntaxStatus {
  if (!value.trim()) return { ok: true, message: '空脚本' };
  try {
    luaparse.parse(value, {
      comments: false,
      locations: true,
      luaVersion: '5.3',
      ranges: true,
      scope: true,
    });
    return { ok: true, message: 'Lua 语法检查通过' };
  } catch (error) {
    const item = error as { message?: string; line?: number; column?: number };
    return {
      ok: false,
      message: item.message || 'Lua 语法错误',
      line: item.line,
      column: item.column,
    };
  }
}

function luaSyntaxLinter() {
  return linter((view): Diagnostic[] => {
    const value = view.state.doc.toString();
    if (!value.trim()) return [];
    try {
      luaparse.parse(value, {
        comments: false,
        locations: true,
        luaVersion: '5.3',
        ranges: true,
        scope: true,
      });
      return [];
    } catch (error) {
      const item = error as { message?: string; index?: number };
      const from = Math.max(0, Math.min(item.index ?? 0, view.state.doc.length));
      return [{
        from,
        to: Math.min(from + 1, view.state.doc.length),
        severity: 'error',
        message: item.message || 'Lua 语法错误',
      }];
    }
  }, { delay: 250 });
}

export function LuaConfigCodeEditor({ value, variables, onChange, onHint }: Props) {
  const [draftValue, setDraftValue] = useState(value);
  const [syntaxStatus, setSyntaxStatus] = useState<LuaSyntaxStatus>(() => parseLuaSyntax(value));
  const editorViewRef = useRef<EditorView | null>(null);
  const completionTimerRef = useRef<number | null>(null);
  const syncTimerRef = useRef<number | null>(null);
  const lastSyncedValueRef = useRef(value);
  const onHintRef = useRef(onHint);
  const variableSignature = useMemo(() => JSON.stringify(variables), [variables]);
  const stableVariables = useMemo(() => variables, [variableSignature]);
  const openNativeCompletion = (view?: EditorView) => {
    const target = view ?? editorViewRef.current;
    if (!target) return;
    startCompletion(target);
    if (completionTimerRef.current !== null) window.clearTimeout(completionTimerRef.current);
    completionTimerRef.current = window.setTimeout(() => {
      startCompletion(target);
    }, 120);
  };
  const flushChange = (nextValue = draftValue) => {
    if (syncTimerRef.current !== null) {
      window.clearTimeout(syncTimerRef.current);
      syncTimerRef.current = null;
    }
    if (nextValue === lastSyncedValueRef.current) return;
    lastSyncedValueRef.current = nextValue;
    onChange(nextValue);
  };
  const scheduleChange = (nextValue: string) => {
    if (syncTimerRef.current !== null) window.clearTimeout(syncTimerRef.current);
    syncTimerRef.current = window.setTimeout(() => flushChange(nextValue), 180);
  };
  const extensions = useMemo(() => [
    StreamLanguage.define(lua),
    autocompletion({
      override: [luaCompletionSource(stableVariables)],
      activateOnTyping: true,
      activateOnTypingDelay: 0,
      interactionDelay: 0,
      closeOnBlur: false,
    }),
    keymap.of(completionKeymap),
    lintGutter(),
    luaSyntaxLinter(),
    EditorView.updateListener.of((update) => {
      if (!update.docChanged) return;
      if (shouldTriggerLuaCompletion(update.view)) {
        onHintRef.current();
        openNativeCompletion(update.view);
      }
    }),
  ], [stableVariables]);

  useEffect(() => {
    if (value === lastSyncedValueRef.current || value === draftValue) return;
    lastSyncedValueRef.current = value;
    setDraftValue(value);
  }, [draftValue, value]);

  useEffect(() => {
    onHintRef.current = onHint;
  }, [onHint]);

  useEffect(() => () => {
    if (completionTimerRef.current !== null) window.clearTimeout(completionTimerRef.current);
    if (syncTimerRef.current !== null) window.clearTimeout(syncTimerRef.current);
  }, []);

  return (
    <div className="node-lua-code-shell" onFocus={onHint} onBlur={() => flushChange()}>
      <CodeMirror
        value={draftValue}
        height="180px"
        onCreateEditor={(view) => {
          editorViewRef.current = view;
        }}
        basicSetup={{ lineNumbers: true, foldGutter: true, highlightActiveLine: true, autocompletion: false }}
        extensions={extensions}
        placeholder="scope = 'USER'\nmaxCount = Resource.default.get('daily_checkin_max_count')\nconsumeMode = 'ATOMIC'"
        onChange={(nextValue) => {
          setDraftValue(nextValue);
          setSyntaxStatus(parseLuaSyntax(nextValue));
          scheduleChange(nextValue);
        }}
      />
      <div className={syntaxStatus.ok ? 'node-lua-syntax-status ok' : 'node-lua-syntax-status error'}>
        <span>{syntaxStatus.ok ? '语法正常' : '语法错误'}</span>
        <em>
          {syntaxStatus.ok
            ? syntaxStatus.message
            : `${syntaxStatus.line ?? '-'}:${syntaxStatus.column ?? '-'} ${syntaxStatus.message}`}
        </em>
      </div>
    </div>
  );
}
