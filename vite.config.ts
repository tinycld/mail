import { one } from 'one/vite'
import { defineConfig } from 'vite'

export default defineConfig({
    server: {
        port: Number(process.env.VITE_PORT || 7100),
    },
    plugins: [
        one({
            web: {
                defaultRenderMode: 'spa',
            },

            ...(process.env.TEST_METRO && {
                native: {
                    bundler: 'metro',
                },
            }),
        }),
    ],
})
