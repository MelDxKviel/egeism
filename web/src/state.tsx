import { createContext, useContext, useEffect, useState, ReactNode, useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Role, SubjectCode, User, api, setToken, clearToken, hasToken } from "./api";

export type View =
  | "dashboard" | "subject" | "solve" | "results" | "history"
  | "t-dashboard" | "t-class" | "t-student" | "t-builder" | "t-assign" | "t-bank" | "t-test"
  | "a-stats" | "a-users"
  | "profile";

interface AppState {
  theme: "light" | "dark";
  subject: SubjectCode;
  view: View;
  user?: User;          // authenticated user; undefined => show login
  role?: Role;          // derived from the account
  ready: boolean;       // auth check finished
  toast?: string;
  setTheme: (t: "light" | "dark") => void;
  setSubject: (s: SubjectCode) => void;
  go: (v: View) => void;
  showToast: (m: string) => void;
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
}

const Ctx = createContext<AppState | null>(null);
export const useApp = () => {
  const c = useContext(Ctx);
  if (!c) throw new Error("useApp outside provider");
  return c;
};

const homeView = (r: Role): View =>
  r === "admin" ? "a-stats" : r === "teacher" ? "t-dashboard" : "dashboard";

// Nav tabs that are safe to restore after a reload (solve/results and the
// detail pages are excluded — their handoff state doesn't survive a refresh).
const RESTORABLE: Record<Role, View[]> = {
  student: ["dashboard", "subject", "history", "profile"],
  teacher: ["t-dashboard", "t-builder", "t-assign", "t-bank", "profile"],
  admin: ["a-stats", "a-users", "profile"],
};
const TAB_VIEWS: View[] = [...new Set([...RESTORABLE.student, ...RESTORABLE.teacher, ...RESTORABLE.admin])];

// restoreView returns the saved tab if it's valid for the role, else the home.
function restoreView(role: Role): View {
  const saved = localStorage.getItem("egeism.view") as View | null;
  if (saved && RESTORABLE[role]?.includes(saved)) return saved;
  return homeView(role);
}

// initialSubject respects a subject-scoped teacher: their one subject wins over
// whatever an earlier account left in localStorage.
function initialSubject(u: User): SubjectCode {
  if (u.role === "teacher" && u.subject) return u.subject;
  return (localStorage.getItem("egeism.subject") as SubjectCode) || "math";
}

export function AppProvider({ children }: { children: ReactNode }) {
  // Most query keys aren't user-scoped, and staleTime is 30s — wipe the cache
  // whenever the acting account changes so a logout→login on one browser never
  // serves the previous user's roster/classes/profile.
  const queryClient = useQueryClient();
  const [theme, setThemeS] = useState<"light" | "dark">(
    (localStorage.getItem("egeism.theme") as "light" | "dark") || "light");
  const [subject, setSubjectS] = useState<SubjectCode>(
    (localStorage.getItem("egeism.subject") as SubjectCode) || "math");
  const [user, setUser] = useState<User | undefined>();
  const [view, setView] = useState<View>("dashboard");
  const [ready, setReady] = useState(false);
  const [toast, setToast] = useState<string>();

  // Restore session from a stored token on load.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      if (!hasToken()) { setReady(true); return; }
      try {
        const me = await api.me();
        if (cancelled) return;
        setUser(me);
        setSubjectS(initialSubject(me));
        setView(restoreView(me.role));
      } catch {
        clearToken();
      } finally {
        if (!cancelled) setReady(true);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  const setTheme = (t: "light" | "dark") => { setThemeS(t); localStorage.setItem("egeism.theme", t); };
  const setSubject = (s: SubjectCode) => { setSubjectS(s); localStorage.setItem("egeism.subject", s); };
  const showToast = useCallback((m: string) => { setToast(m); setTimeout(() => setToast(undefined), 2600); }, []);
  // Persist only real nav tabs, so diving into solve doesn't clobber the tab
  // the user returns to after a reload.
  const go = (v: View) => {
    setView(v);
    if (TAB_VIEWS.includes(v)) localStorage.setItem("egeism.view", v);
  };

  const applyAuth = (res: { token: string; user: User }) => {
    queryClient.clear();
    setToken(res.token);
    setUser(res.user);
    setSubjectS(initialSubject(res.user));
    const v = homeView(res.user.role);
    setView(v);
    localStorage.setItem("egeism.view", v);
  };
  const login = async (username: string, password: string) => { applyAuth(await api.login(username, password)); };
  const logout = () => { clearToken(); setUser(undefined); queryClient.clear(); };

  return (
    <Ctx.Provider value={{
      theme, subject, view, user, role: user?.role, ready, toast,
      setTheme, setSubject, go, showToast, login, logout,
    }}>
      {children}
    </Ctx.Provider>
  );
}
