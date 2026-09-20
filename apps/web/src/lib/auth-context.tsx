"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

import * as api from "./api";

const STORAGE_KEY = "polaris.auth.token";

type AuthState = {
  user: api.User | null;
  token: string | null;
  loading: boolean;
};

type AuthContextValue = AuthState & {
  login: (email: string, password: string) => Promise<void>;
  register: (name: string, email: string, password: string) => Promise<void>;
  logout: () => void;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ user: null, token: null, loading: true });

  useEffect(() => {
    const token = localStorage.getItem(STORAGE_KEY);
    const resolveUser = token ? api.me(token) : Promise.resolve(null);
    resolveUser
      .then((user) => setState(user ? { user, token: token!, loading: false } : { user: null, token: null, loading: false }))
      .catch(() => {
        localStorage.removeItem(STORAGE_KEY);
        setState({ user: null, token: null, loading: false });
      });
  }, []);

  function persist(user: api.User, token: string) {
    localStorage.setItem(STORAGE_KEY, token);
    setState({ user, token, loading: false });
  }

  async function login(email: string, password: string) {
    const res = await api.login(email, password);
    persist(res.user, res.token);
  }

  async function register(name: string, email: string, password: string) {
    const res = await api.register(name, email, password);
    persist(res.user, res.token);
  }

  function logout() {
    localStorage.removeItem(STORAGE_KEY);
    setState({ user: null, token: null, loading: false });
  }

  return (
    <AuthContext.Provider value={{ ...state, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
