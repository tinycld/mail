const DEV_FALLBACK = 'http://127.0.0.1:7090'

export const PB_SERVER_ADDR =
    process.env.VITE_PB_SERVER_ADDR ||
    process.env.PB_SERVER_ADDR ||
    (typeof window !== 'undefined' ? window.location.origin : DEV_FALLBACK)
