import type {
  CampaignBlock,
  CampaignBlockKind,
  CampaignPageSchema,
  DeviceWidth,
  FlexAlign,
  FlexDirection,
  FlexJustify,
  FlexWrap,
  FreeDesignElement,
  LayoutKind,
} from './types';
import { blockIdForFreeElementId, blockToFreeKind, blockToFreeText, blocksToFreeElements, normalizeLayoutKind } from './layout';
import { componentDefinitionForElement, componentKeyForBlockKind, componentKeyForLayoutKind } from './componentRegistry';

export type NodeId = string;

export type ResourceBinding = {
  id: string;
  resourceId?: string;
  expression: string;
  fallback?: string;
};

export type ActionBinding = {
  id: string;
  actionId: string;
  trigger: 'click' | 'exposure' | 'submit' | 'timer';
  payloadExpression?: string;
};

export type StyleToken = {
  id: string;
  name: string;
  token: string;
  value: string;
  scope?: 'page' | 'component' | 'runtime';
};

export type NodeFrame = {
  x: number;
  y: number;
  width: number;
  height: number;
  z: number;
};

type NodeBase = {
  id: NodeId;
  name: string;
  visible: boolean;
  parentId?: NodeId;
  frame: NodeFrame;
  radius?: string;
  resourceBindings: ResourceBinding[];
  actionBindings: ActionBinding[];
  styleTokens: StyleToken[];
};

export type ContainerNode = NodeBase & {
  type: 'container';
  layout: LayoutKind;
  children: NodeId[];
  content: {
    text: string;
  };
  flexDirection: FlexDirection;
  justifyContent: FlexJustify;
  alignItems: FlexAlign;
  flexWrap: FlexWrap;
  gap: number;
};

export type ComponentKind = CampaignBlockKind | 'Text' | 'Image' | 'Card' | 'Button';

export type ComponentNode = NodeBase & {
  type: 'component';
  component: ComponentKind;
  content: {
    text: string;
    variant?: 'primary' | 'secondary';
  };
  sourceBlockId?: string;
};

export type Node = ContainerNode | ComponentNode;

export type Page = {
  schemaVersion: 1;
  id: string;
  activityId: string;
  name: string;
  path: string;
  device: {
    defaultWidth: DeviceWidth;
    supportedWidths: DeviceWidth[];
  };
  rootNodeIds: NodeId[];
  nodes: Node[];
  resourceBindings: ResourceBinding[];
  actionBindings: ActionBinding[];
  styleTokens: StyleToken[];
};

type CampaignPageLike = Omit<CampaignPageSchema, 'editor'> & Partial<Pick<CampaignPageSchema, 'editor'>>;

function bindingFromExpression(nodeId: string, expression?: string): ResourceBinding[] {
  return expression ? [{ id: `${nodeId}_resource`, expression }] : [];
}

function actionFromElement(nodeId: string, actionId?: string, payloadExpression?: string): ActionBinding[] {
  return actionId ? [{ id: `${nodeId}_action`, actionId, trigger: 'click', payloadExpression }] : [];
}

function styleTokensFromElement(nodeId: string, element: FreeDesignElement): StyleToken[] {
  const tokens: StyleToken[] = [];
  if (element.radius !== undefined) {
    tokens.push({ id: `${nodeId}_radius`, name: '圆角', token: '--campaign-node-radius', value: element.radius, scope: 'component' });
  }
  if (element.variant) {
    tokens.push({ id: `${nodeId}_variant`, name: '样式变体', token: '--campaign-node-variant', value: element.variant, scope: 'component' });
  }
  return tokens;
}

function componentKindForElement(element: FreeDesignElement, block?: CampaignBlock): ComponentKind {
  if (block) return block.kind;
  if (element.component === 'Button') return 'Button';
  if (element.component === 'Image') return 'Image';
  if (element.component === 'Title') return 'Text';
  if (element.component === 'HeroBanner' || element.component === 'RewardCard') return element.component;
  if (element.kind === 'text') return 'Text';
  if (element.kind === 'image') return 'Image';
  if (element.kind === 'button') return 'Button';
  return 'Card';
}

function blockForElement(element: FreeDesignElement, blocksById: Map<string, CampaignBlock>) {
  const blockId = blockIdForFreeElementId(element.id);
  return blockId ? blocksById.get(blockId) : undefined;
}

export function campaignPageToEditorPage(page: CampaignPageLike): Page {
  const fallbackElements = page.freeElements?.length ? page.freeElements : blocksToFreeElements(page.blocks, page.device.defaultWidth);
  const elements = [...fallbackElements].sort((a, b) => a.z - b.z);
  const blocksById = new Map(page.blocks.map((block) => [block.id, block]));
  const childrenByParent = elements.reduce<Record<string, string[]>>((groups, element) => {
    if (!element.parentId) return groups;
    return { ...groups, [element.parentId]: [...(groups[element.parentId] ?? []), element.id] };
  }, {});
  const nodes: Node[] = elements.map((element) => {
    const frame: NodeFrame = { x: element.x, y: element.y, width: element.w, height: element.h, z: element.z };
    const block = blockForElement(element, blocksById);
    const definition = componentDefinitionForElement(element);
    const name = element.text.split('\\n')[0] || block?.title || definition.defaultTitle || '未命名节点';
    const resourceBindings = bindingFromExpression(element.id, element.binding);
    const actionBindings = actionFromElement(element.id, element.action, element.payload);
    const styleTokens = styleTokensFromElement(element.id, element);
    if (element.kind === 'layout') {
      const layout = normalizeLayoutKind(element.layout);
      return {
        id: element.id,
        type: 'container',
        name,
        visible: true,
        parentId: element.parentId,
        frame,
        radius: element.radius,
        resourceBindings,
        actionBindings,
        styleTokens,
        layout,
        children: childrenByParent[element.id] ?? [],
        content: {
          text: element.text,
        },
        flexDirection: element.flexDirection ?? (layout === 'vertical-layout' ? 'column' : 'row'),
        justifyContent: element.justifyContent ?? 'flex-start',
        alignItems: element.alignItems ?? 'stretch',
        flexWrap: element.flexWrap ?? 'nowrap',
        gap: element.gap ?? 8,
      };
    }
    return {
      id: element.id,
      type: 'component',
      name: block?.title ?? name,
      visible: block?.visible ?? true,
      parentId: element.parentId,
      frame,
      radius: element.radius,
      resourceBindings,
      actionBindings,
      styleTokens,
      component: componentKindForElement(element, block),
      sourceBlockId: block?.id,
      content: {
        text: element.text,
        variant: element.variant,
      },
    };
  });
  return {
    schemaVersion: 1,
    id: page.id,
    activityId: page.activityId,
    name: page.name,
    path: page.path,
    device: page.device,
    rootNodeIds: elements.filter((element) => !element.parentId).map((element) => element.id),
    nodes,
    resourceBindings: nodes.flatMap((node) => node.resourceBindings),
    actionBindings: nodes.flatMap((node) => node.actionBindings),
    styleTokens: nodes.flatMap((node) => node.styleTokens),
  };
}

export function editorPageToFreeElements(editorPage: Page): FreeDesignElement[] {
  return editorPage.nodes.map((node) => ({
    id: node.id,
    kind: node.type === 'container'
      ? 'layout'
      : node.component === 'Text' || node.component === 'Title'
        ? 'text'
        : node.component === 'Image'
          ? 'image'
          : node.component === 'Button' || node.component === 'ClaimButton'
            ? 'button'
            : 'card',
    text: node.type === 'component' ? node.content.text : node.content.text,
    component: node.type === 'container'
      ? componentKeyForLayoutKind(node.layout)
      : node.sourceBlockId
        ? componentKeyForBlockKind(node.component as CampaignBlockKind)
        : node.component === 'Text'
          ? 'Title'
          : node.component === 'Button'
            ? 'Button'
            : node.component === 'Image'
              ? 'Image'
              : node.component === 'HeroBanner'
                ? 'HeroBanner'
                : 'RewardCard',
    x: node.frame.x,
    y: node.frame.y,
    w: node.frame.width,
    h: node.frame.height,
    z: node.frame.z,
    parentId: node.parentId,
    binding: node.resourceBindings[0]?.expression,
    action: node.actionBindings[0]?.actionId,
    payload: node.actionBindings[0]?.payloadExpression,
    variant: node.type === 'component' ? node.content.variant : undefined,
    layout: node.type === 'container' ? node.layout : undefined,
    flexDirection: node.type === 'container' ? node.flexDirection : undefined,
    justifyContent: node.type === 'container' ? node.justifyContent : undefined,
    alignItems: node.type === 'container' ? node.alignItems : undefined,
    flexWrap: node.type === 'container' ? node.flexWrap : undefined,
    gap: node.type === 'container' ? node.gap : undefined,
    radius: node.radius,
  }));
}

export function editorPageToBlocks(editorPage: Page, currentBlocks: CampaignBlock[]): CampaignBlock[] {
  const blocksById = new Map(currentBlocks.map((block) => [block.id, block]));
  return editorPage.nodes.flatMap((node): CampaignBlock[] => {
    if (node.type !== 'component' || !node.sourceBlockId) return [];
    const current = blocksById.get(node.sourceBlockId);
    if (!current) return [];
    return [{
      ...current,
      title: node.name,
      visible: node.visible,
      text: node.component === 'ClaimButton' ? node.content.text : current.text,
      binding: node.resourceBindings[0]?.expression,
      action: node.actionBindings[0]?.actionId,
      payload: node.actionBindings[0]?.payloadExpression,
      variant: node.content.variant,
      radius: node.radius,
    }];
  });
}

function hasEditorPage(page: CampaignPageLike): page is CampaignPageLike & { editor: Page } {
  return Boolean(
    page.editor &&
    page.editor.schemaVersion === 1 &&
    Array.isArray(page.editor.rootNodeIds) &&
    Array.isArray(page.editor.nodes),
  );
}

function withExistingEditorPage(page: CampaignPageLike & { editor: Page }): Page {
  const fallback = campaignPageToEditorPage(page);
  const existingNodes = new Map(page.editor.nodes.map((node) => [node.id, node]));
  const fallbackElements = page.freeElements?.length ? page.freeElements : blocksToFreeElements(page.blocks, page.device.defaultWidth);
  const fallbackElementsById = new Map(fallbackElements.map((element) => [element.id, element]));
  const hasLegacyClear = (nodeId: NodeId, field: 'binding' | 'action') => {
    const element = fallbackElementsById.get(nodeId);
    return element ? Object.prototype.hasOwnProperty.call(element, field) && element[field] === '' : false;
  };
  const mergedResourceBindings = (existingNode: Node, fallbackNode: Node) => {
    const existingFirst = existingNode.resourceBindings?.[0];
    const fallbackFirst = fallbackNode.resourceBindings[0];
    if (!fallbackFirst) return hasLegacyClear(fallbackNode.id, 'binding') ? [] : existingNode.resourceBindings ?? [];
    if (!existingFirst || existingFirst.expression !== fallbackFirst.expression) return fallbackNode.resourceBindings;
    return existingNode.resourceBindings ?? fallbackNode.resourceBindings;
  };
  const mergedActionBindings = (existingNode: Node, fallbackNode: Node) => {
    const existingFirst = existingNode.actionBindings?.[0];
    const fallbackFirst = fallbackNode.actionBindings[0];
    if (!fallbackFirst) return hasLegacyClear(fallbackNode.id, 'action') ? [] : existingNode.actionBindings ?? [];
    if (!existingFirst || existingFirst.actionId !== fallbackFirst.actionId || existingFirst.payloadExpression !== fallbackFirst.payloadExpression) return fallbackNode.actionBindings;
    return existingNode.actionBindings ?? fallbackNode.actionBindings;
  };
  const nodes = fallback.nodes.map((fallbackNode) => {
    const existingNode = existingNodes.get(fallbackNode.id);
    if (!existingNode || existingNode.type !== fallbackNode.type) return fallbackNode;
    const base = {
      ...existingNode,
      name: fallbackNode.name || existingNode.name || '未命名节点',
      visible: fallbackNode.visible,
      parentId: fallbackNode.parentId,
      frame: fallbackNode.frame,
      radius: fallbackNode.radius,
      resourceBindings: mergedResourceBindings(existingNode, fallbackNode),
      actionBindings: mergedActionBindings(existingNode, fallbackNode),
      styleTokens: existingNode.styleTokens ?? fallbackNode.styleTokens,
    };
    if (base.type !== 'container') {
      const fallbackComponent = fallbackNode.type === 'component' ? fallbackNode : undefined;
      return fallbackComponent ? {
        ...base,
        component: fallbackComponent.component,
        sourceBlockId: fallbackComponent.sourceBlockId,
        content: fallbackComponent.content,
      } : base;
    }
    if (fallbackNode.type !== 'container') return fallbackNode;
    const fallbackContainer = fallbackNode;
    return {
      ...base,
      layout: fallbackContainer.layout,
      children: fallbackContainer.children,
      content: fallbackContainer.content,
      flexDirection: fallbackContainer.flexDirection,
      justifyContent: fallbackContainer.justifyContent,
      alignItems: fallbackContainer.alignItems,
      flexWrap: fallbackContainer.flexWrap,
      gap: fallbackContainer.gap,
    };
  });
  const fallbackRootIds = new Set(fallback.rootNodeIds);
  const rootNodeIds = [
    ...page.editor.rootNodeIds.filter((id) => fallbackRootIds.has(id)),
    ...fallback.rootNodeIds.filter((id) => !page.editor.rootNodeIds.includes(id)),
  ];
  return {
    ...fallback,
    ...page.editor,
    schemaVersion: 1,
    id: page.id,
    activityId: page.activityId,
    name: page.name,
    path: page.path,
    device: page.device,
    rootNodeIds,
    nodes,
    resourceBindings: page.editor.resourceBindings ?? nodes.flatMap((node) => node.resourceBindings),
    actionBindings: page.editor.actionBindings ?? nodes.flatMap((node) => node.actionBindings),
    styleTokens: page.editor.styleTokens ?? nodes.flatMap((node) => node.styleTokens),
  };
}

export function withEditorPage(page: CampaignPageLike): CampaignPageSchema {
  return {
    ...page,
    editor: hasEditorPage(page) ? withExistingEditorPage(page) : campaignPageToEditorPage(page),
  };
}

export function defaultEditorPageFromBlocks(page: CampaignPageLike): Page {
  const freeElements = page.freeElements?.length ? page.freeElements : blocksToFreeElements(page.blocks, page.device.defaultWidth);
  return campaignPageToEditorPage({ ...page, freeElements });
}

export function nodeToFreeElementText(node: Node) {
  if (node.type === 'component') return node.content.text;
  return node.content.text;
}

export function blockToDefaultComponentText(block: CampaignBlock) {
  return blockToFreeText(block);
}

export function blockToDefaultComponentKind(block: CampaignBlock): ComponentKind {
  const freeKind = blockToFreeKind(block.kind);
  if (freeKind === 'text') return 'Text';
  if (freeKind === 'button') return 'Button';
  return block.kind;
}
