import { create } from 'zustand';
import type { Thread, ThreadItem, StreamEvent, ApprovalRequest, QuestionRequest, SkillInfo, ServerSettings } from "@/features/rpc/types";
import type { TurnStatus } from "./chatUtils";
import type { NavView } from "./IconNav";

interface ChatState {
  activeView: NavView;
  setActiveView: (view: NavView) => void;
  
  threads: Thread[];
  setThreads: (threads: Thread[] | ((prev: Thread[]) => Thread[])) => void;
  
  activeThreadId: string;
  setActiveThreadId: (id: string) => void;
  
  messages: ThreadItem[];
  setMessages: (messages: ThreadItem[] | ((prev: ThreadItem[]) => ThreadItem[])) => void;
  
  streamText: string;
  setStreamText: (text: string | ((prev: string) => string)) => void;
  
  turnStatus: TurnStatus;
  setTurnStatus: (status: TurnStatus) => void;
  
  toolEvents: StreamEvent[];
  setToolEvents: (events: StreamEvent[] | ((prev: StreamEvent[]) => StreamEvent[])) => void;
  
  approvals: ApprovalRequest[];
  setApprovals: (approvals: ApprovalRequest[] | ((prev: ApprovalRequest[]) => ApprovalRequest[])) => void;
  
  questions: QuestionRequest[];
  setQuestions: (questions: QuestionRequest[] | ((prev: QuestionRequest[]) => QuestionRequest[])) => void;
  
  panelCollapsed: boolean;
  setPanelCollapsed: (collapsed: boolean | ((prev: boolean) => boolean)) => void;
  
  mobileDrawerOpen: boolean;
  setMobileDrawerOpen: (open: boolean | ((prev: boolean) => boolean)) => void;
  
  canvasChatOpen: boolean;
  setCanvasChatOpen: (open: boolean | ((prev: boolean) => boolean)) => void;

  wsConnected: boolean;
  setWsConnected: (connected: boolean) => void;
  
  wsHasBeenConnected: boolean;
  setWsHasBeenConnected: (hasBeen: boolean) => void;

  skills: SkillInfo[];
  setSkills: (skills: SkillInfo[]) => void;

  settings: ServerSettings | null;
  setSettings: (settings: ServerSettings | null | ((prev: ServerSettings | null) => ServerSettings | null)) => void;

  activeTurnId: string;
  setActiveTurnId: (id: string) => void;

  registeredTools: { name: string; description: string; category: string }[];
  setRegisteredTools: (tools: { name: string; description: string; category: string }[]) => void;

  embedBackends: { name: string; env_key: string; available: boolean }[];
  setEmbedBackends: (backends: { name: string; env_key: string; available: boolean }[]) => void;

  currentUser: { username: string; role: string };
  setCurrentUser: (user: { username: string; role: string }) => void;

  bootstrapped: boolean;
  setBootstrapped: (bootstrapped: boolean) => void;

  showLogin: boolean;
  setShowLogin: (show: boolean) => void;

  serverReachable: boolean;
  setServerReachable: (reachable: boolean) => void;
}

export const useChatStore = create<ChatState>((set) => ({
  activeView: "chats",
  setActiveView: (activeView) => set({ activeView }),
  
  threads: [],
  setThreads: (threads) => set((state) => ({ threads: typeof threads === 'function' ? threads(state.threads) : threads })),
  
  activeThreadId: "",
  setActiveThreadId: (activeThreadId) => set({ activeThreadId }),
  
  messages: [],
  setMessages: (messages) => set((state) => ({ messages: typeof messages === 'function' ? messages(state.messages) : messages })),
  
  streamText: "",
  setStreamText: (streamText) => set((state) => ({ streamText: typeof streamText === 'function' ? streamText(state.streamText) : streamText })),
  
  turnStatus: "idle",
  setTurnStatus: (turnStatus) => set({ turnStatus }),
  
  toolEvents: [],
  setToolEvents: (toolEvents) => set((state) => ({ toolEvents: typeof toolEvents === 'function' ? toolEvents(state.toolEvents) : toolEvents })),
  
  approvals: [],
  setApprovals: (approvals) => set((state) => ({ approvals: typeof approvals === 'function' ? approvals(state.approvals) : approvals })),
  
  questions: [],
  setQuestions: (questions) => set((state) => ({ questions: typeof questions === 'function' ? questions(state.questions) : questions })),
  
  panelCollapsed: true,
  setPanelCollapsed: (panelCollapsed) => set((state) => ({ panelCollapsed: typeof panelCollapsed === 'function' ? panelCollapsed(state.panelCollapsed) : panelCollapsed })),
  
  mobileDrawerOpen: false,
  setMobileDrawerOpen: (mobileDrawerOpen) => set((state) => ({ mobileDrawerOpen: typeof mobileDrawerOpen === 'function' ? mobileDrawerOpen(state.mobileDrawerOpen) : mobileDrawerOpen })),
  
  canvasChatOpen: false,
  setCanvasChatOpen: (canvasChatOpen) => set((state) => ({ canvasChatOpen: typeof canvasChatOpen === 'function' ? canvasChatOpen(state.canvasChatOpen) : canvasChatOpen })),

  wsConnected: false,
  setWsConnected: (wsConnected) => set({ wsConnected }),

  wsHasBeenConnected: false,
  setWsHasBeenConnected: (wsHasBeenConnected) => set({ wsHasBeenConnected }),

  skills: [],
  setSkills: (skills) => set({ skills }),

  settings: null,
  setSettings: (settings) => set((state) => ({ settings: typeof settings === 'function' ? settings(state.settings) : settings })),

  activeTurnId: "",
  setActiveTurnId: (activeTurnId) => set({ activeTurnId }),

  registeredTools: [],
  setRegisteredTools: (registeredTools) => set({ registeredTools }),

  embedBackends: [],
  setEmbedBackends: (embedBackends) => set({ embedBackends }),

  currentUser: { username: "", role: "admin" },
  setCurrentUser: (currentUser) => set({ currentUser }),

  bootstrapped: false,
  setBootstrapped: (bootstrapped) => set({ bootstrapped }),

  showLogin: false,
  setShowLogin: (showLogin) => set({ showLogin }),

  serverReachable: true,
  setServerReachable: (serverReachable) => set({ serverReachable }),
}));
