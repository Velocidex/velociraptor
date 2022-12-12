import React from 'react';

import VeloReportViewer from "../artifacts/reporting.jsx";

export default class Welcome extends React.Component {
    render() {
        return (
            <VeloReportViewer
              type="CLIENT"
              artifact="Custom.Server.Internal.Welcome" />
        );
    }
};
