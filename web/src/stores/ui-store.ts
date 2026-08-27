import { create } from 'zustand'

type Theme = 'light' | 'dark'

interface UIState {
  sidebarOpen: boolean
  commandOpen: boolean
  notificationsOpen: boolean
  accessOpen: boolean
  theme: Theme
  setSidebarOpen: (open: boolean) => void
  setCommandOpen: (open: boolean) => void
  setNotificationsOpen: (open: boolean) => void
  setAccessOpen: (open: boolean) => void
  toggleTheme: () => void
}

const savedTheme = (sessionStorage.getItem('adminTheme') as Theme | null) ?? 'light'

export const useUIStore = create<UIState>((set, get) => ({
  sidebarOpen: false,
  commandOpen: false,
  notificationsOpen: false,
  accessOpen: false,
  theme: savedTheme,
  setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
  setCommandOpen: (commandOpen) => set({ commandOpen }),
  setNotificationsOpen: (notificationsOpen) => set({ notificationsOpen }),
  setAccessOpen: (accessOpen) => set({ accessOpen }),
  toggleTheme: () => {
    const theme = get().theme === 'light' ? 'dark' : 'light'
    sessionStorage.setItem('adminTheme', theme)
    set({ theme })
  },
}))
