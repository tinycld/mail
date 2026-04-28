type Listener = () => void

const listeners = new Set<Listener>()

export const composeEvents = {
    emit() {
        listeners.forEach(listener => {
            listener()
        })
    },
    subscribe(listener: Listener) {
        listeners.add(listener)
        return () => {
            listeners.delete(listener)
        }
    },
}
