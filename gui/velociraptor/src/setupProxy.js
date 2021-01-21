const { createProxyMiddleware } = require('http-proxy-middleware');

module.exports = function(app) {
    let mw = createProxyMiddleware({
        target: 'https://0.0.0.0:8889',
        changeOrigin: true,
        secure: false,
    });

    app.use('/api', mw);
    app.use('/notebooks', mw);
    app.use('/downloads', mw);

    let appHandlers = createProxyMiddleware({
        target: 'https://0.0.0.0:8889/app',
        changeOrigin: true,
        secure: false,
    });
    app.use('/assets', appHandlers);
};
