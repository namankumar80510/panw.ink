// Reader SPA entrypoint. Bootstrap CSS/JS and Bootstrap Icons are installed
// npm packages bundled by Parcel; nothing is loaded from a CDN.
import 'bootstrap/dist/css/bootstrap.min.css';
import 'bootstrap-icons/font/bootstrap-icons.css';
import './styles/app.css';
import m from 'mithril';
import { routes } from './router';
import { config } from './stores/config';
import { prefs } from './stores/prefs';
import { session } from './stores/session';

prefs.apply();
m.route.prefix = '';
void config.load().then(() => m.redraw());
void session.load();
m.route(document.getElementById('app')!, '/', routes);
