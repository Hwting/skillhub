"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { authApi, setOnUnauthorized } from "@/lib/api";
import type { User } from "@/lib/types";

interface UserContextValue {
  user: User | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<User>;
  register: (email: string, username: string, password: string) => Promise<User>;
  logout: () => Promise<void>;
  setUser: (u: User | null) => void;
}

const UserContext = createContext<UserContextValue | null>(null);

export function UserProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // Clearing the user on a 401 is the right action everywhere: protected
    // pages redirect via AuthGuard when user becomes null; /login and /register
    // are unaffected. Avoids a hard router.push here.
    setOnUnauthorized(() => setUser(null));
    return () => setOnUnauthorized(null);
  }, []);

  useEffect(() => {
    let cancelled = false;
    authApi
      .me()
      .then((u) => {
        if (!cancelled) setUser(u);
      })
      .catch(() => {
        // 401 or worse — not logged in.
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const login = async (email: string, password: string) => {
    const u = await authApi.login({ email, password });
    setUser(u);
    return u;
  };

  const register = async (email: string, username: string, password: string) => {
    const u = await authApi.register({ email, username, password });
    setUser(u);
    return u;
  };

  const logout = async () => {
    await authApi.logout();
    setUser(null);
  };

  return (
    <UserContext.Provider value={{ user, loading, login, register, logout, setUser }}>
      {children}
    </UserContext.Provider>
  );
}

export function useUser(): UserContextValue {
  const ctx = useContext(UserContext);
  if (!ctx) throw new Error("useUser must be used within UserProvider");
  return ctx;
}
