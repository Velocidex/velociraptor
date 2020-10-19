import React from 'react';

import VeloReportViewer from "../artifacts/reporting.js";

export default class Welcome extends React.Component {
    render() {
        return (
            <VeloReportViewer artifact="Custom.Server.Internal.Welcome" />
        );
    }
};
