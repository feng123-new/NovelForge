import { initializeLocale } from './lib/i18n';
import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';

const target = document.getElementById('app');
if (!target) throw new Error('NovelForge workspace mount point is missing');

initializeLocale();
mount(App, { target });
