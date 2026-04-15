import React from 'react';
import { createRoot } from 'react-dom/client';
import { Demo } from './Demo';

const container = document.getElementById('root');
if (!container) throw new Error('root element missing');

createRoot(container).render(
  <React.StrictMode>
    <Demo />
  </React.StrictMode>,
);
