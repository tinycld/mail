import { AudioPreview } from '../components/previews/AudioPreview'
import { CodePreview } from '../components/previews/CodePreview'
import { GenericPreview } from '../components/previews/GenericPreview'
import { ImagePreview } from '../components/previews/ImagePreview'
import { PdfPreview } from '../components/previews/PdfPreview'
import { VideoPreview } from '../components/previews/VideoPreview'
import { registerPreview } from './preview-registry'

registerPreview('image/*', { preview: ImagePreview })
registerPreview('application/pdf', { preview: PdfPreview })
registerPreview('video/*', { preview: VideoPreview })
registerPreview('audio/*', { preview: AudioPreview })
registerPreview('text/javascript', { preview: CodePreview })
registerPreview('application/json', { preview: CodePreview })
registerPreview('text/html', { preview: CodePreview })
registerPreview('text/css', { preview: CodePreview })
registerPreview('text/plain', { preview: CodePreview })
registerPreview('text/csv', { preview: CodePreview })
registerPreview('text/xml', { preview: CodePreview })
registerPreview('application/xml', { preview: CodePreview })
registerPreview('*', { preview: GenericPreview })
