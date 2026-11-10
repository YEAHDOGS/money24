import Feed from './Feed.svelte';
import './feed.css';
import { mount } from 'svelte';

export default mount(Feed, { target: document.getElementById('feed') });
