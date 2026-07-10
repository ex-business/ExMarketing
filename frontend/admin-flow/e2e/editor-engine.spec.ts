import { expect, test } from '@playwright/test';
import { createEditorEngineState, editorEngineReducer } from '../src/campaign-editor/editorEngine';
import type { ContainerNode, Node, Page } from '../src/campaign-editor/editorSchema';

function component(id: string, parentId?: string): Node {
  return {
    id,
    type: 'component',
    component: 'Text',
    name: id,
    visible: true,
    parentId,
    frame: { x: 0, y: 0, width: 200, height: 40, z: 1 },
    positioning: 'flow',
    widthMode: 'fill',
    heightMode: 'auto',
    locked: false,
    radius: '0',
    resourceBindings: [],
    actionBindings: [],
    styleTokens: [],
    content: { text: id },
  };
}

function container(id: string): ContainerNode {
  return {
    ...component(id),
    type: 'container',
    layout: 'vertical-layout',
    children: [],
    content: { text: '' },
    flexDirection: 'column',
    justifyContent: 'flex-start',
    alignItems: 'stretch',
    flexWrap: 'nowrap',
    gap: 8,
  };
}

function page(): Page {
  return {
    schemaVersion: 1,
    id: 'page', activityId: 'activity', name: 'Page', path: '/campaign/page',
    device: { defaultWidth: 393, supportedWidths: [375, 393, 414] },
    rootNodeIds: ['a', 'box', 'b'],
    nodes: [component('a'), container('box'), component('b')],
    resourceBindings: [], actionBindings: [], styleTokens: [],
  };
}

test.describe('editor engine', () => {
  test('places nodes before, inside and after with one canonical tree', () => {
    let state = createEditorEngineState(page());
    state = editorEngineReducer(state, { type: 'placeNode', nodeId: 'b', parentId: 'box', index: 0 });
    expect(state.page.rootNodeIds).toEqual(['a', 'box']);
    expect((state.page.nodes.find((node) => node.id === 'box') as ContainerNode).children).toEqual(['b']);
    expect(state.page.nodes.find((node) => node.id === 'b')).toMatchObject({ parentId: 'box', positioning: 'flow', widthMode: 'fill' });
    state = editorEngineReducer(state, { type: 'placeNode', nodeId: 'b', index: 0 });
    expect(state.page.rootNodeIds).toEqual(['b', 'a', 'box']);
  });

  test('keeps move and resize in one undo transaction', () => {
    let state = createEditorEngineState(page(), 'a');
    state = editorEngineReducer(state, { type: 'updateNode', nodeId: 'a', patch: { positioning: 'absolute', widthMode: 'fixed', heightMode: 'fixed' } });
    state = editorEngineReducer(state, { type: 'beginTransaction' });
    state = editorEngineReducer(state, { type: 'moveNode', nodeId: 'a', position: { x: 80, y: 120 }, transient: true });
    state = editorEngineReducer(state, { type: 'resizeNode', nodeId: 'a', size: { width: 280, height: 90 }, transient: true });
    state = editorEngineReducer(state, { type: 'commitTransaction' });
    expect(state.page.nodes.find((node) => node.id === 'a')?.frame).toMatchObject({ x: 80, y: 120, width: 280, height: 90 });
    state = editorEngineReducer(state, { type: 'undo' });
    expect(state.page.nodes.find((node) => node.id === 'a')?.frame).toMatchObject({ x: 0, y: 0, width: 200, height: 40 });
    state = editorEngineReducer(state, { type: 'redo' });
    expect(state.page.nodes.find((node) => node.id === 'a')?.frame).toMatchObject({ x: 80, y: 120, width: 280, height: 90 });
  });

  test('deletes container descendants and duplicates a full subtree', () => {
    let state = createEditorEngineState(page());
    state = editorEngineReducer(state, { type: 'placeNode', nodeId: 'b', parentId: 'box' });
    state = editorEngineReducer(state, { type: 'duplicateNode', nodeId: 'box' });
    expect(state.page.nodes.filter((node) => node.parentId?.startsWith('box_copy'))).toHaveLength(1);
    state = editorEngineReducer(state, { type: 'deleteNode', nodeId: 'box' });
    expect(state.page.nodes.some((node) => node.id === 'box' || node.id === 'b')).toBe(false);
  });

  test('allocates unique block and binding ids on repeated duplication', () => {
    const source = page();
    source.nodes = source.nodes.map((node) => node.id === 'a' && node.type === 'component' ? {
      ...node,
      sourceBlockId: 'title',
      resourceBindings: [{ id: 'a_resource', resourceId: 'activity_title', expression: '{{ activity.title }}' }],
      actionBindings: [{ id: 'a_action', actionId: 'CLAIM_REWARD', trigger: 'click' }],
      styleTokens: [{ id: 'a_style', name: '颜色', token: '--title-color', value: '#102A56' }],
    } : node);
    let state = createEditorEngineState(source);
    state = editorEngineReducer(state, { type: 'duplicateNode', nodeId: 'a' });
    state = editorEngineReducer(state, { type: 'duplicateNode', nodeId: 'a' });
    const copies = state.page.nodes.filter((node) => node.id === 'a' || node.id.startsWith('a_copy'));
    const sourceBlockIds = copies.flatMap((node) => node.type === 'component' && node.sourceBlockId ? [node.sourceBlockId] : []);
    const resourceBindingIds = copies.flatMap((node) => node.resourceBindings.map((binding) => binding.id));
    const actionBindingIds = copies.flatMap((node) => node.actionBindings.map((binding) => binding.id));
    const styleTokenIds = copies.flatMap((node) => node.styleTokens.map((token) => token.id));
    expect(new Set(sourceBlockIds).size).toBe(sourceBlockIds.length);
    expect(new Set(resourceBindingIds).size).toBe(resourceBindingIds.length);
    expect(new Set(actionBindingIds).size).toBe(actionBindingIds.length);
    expect(new Set(styleTokenIds).size).toBe(styleTokenIds.length);
  });
});
