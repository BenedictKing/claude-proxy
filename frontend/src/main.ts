import { createApp } from 'vue'
import vuetify from './plugins/vuetify'
import App from './App.vue'
import './assets/style.css' // Tailwind + DaisyUI

const app = createApp(App)

app.use(vuetify)

app.mount('#app')
