import './css/_variables.css';
import 'bootstrap/dist/css/bootstrap.css';

import React from 'react';
import { createRoot } from 'react-dom/client';
import './css/index.css';
import Root from './next/Root';
import * as serviceWorker from './serviceWorker';

createRoot(document.getElementById('root')).render(<Root />);

// If you want your app to work offline and load faster, you can change
// unregister() to register() below. Note this comes with some pitfalls.
// Learn more about service workers: https://bit.ly/CRA-PWA
serviceWorker.unregister();
