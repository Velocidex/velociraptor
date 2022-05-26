const { createProxyMiddleware } = require('http-proxy-middleware');


module.exports = function(app) {
    let mw = createProxyMiddleware({
        target: 'https://0.0.0.0:8889',
        changeOrigin: true,
        onProxyReq: (proxyReq, req, res) => {
            proxyReq.setHeader('x-username', 'mic');
        },
        secure: false,
    });

    app.use('/api', mw);
    app.use('/notebooks', mw);
    app.use('/downloads', mw);
    app.use('/hunts', mw);
    app.use('/clients', mw);

    let appHandlers = createProxyMiddleware({
        target: 'https://0.0.0.0:8889/app',
        changeOrigin: true,
        secure: false,
    });
    app.use('/assets', appHandlers);
};
