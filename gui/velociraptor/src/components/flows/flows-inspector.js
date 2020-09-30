import React from 'react';
import PropTypes from 'prop-types';

import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';

import FlowOverview from './flow-overview.js';
import FlowUploads from './flow-uploads.js';
import FlowRequests from './flow-requests.js';
import FlowResults from './flow-results.js';

import { withRouter }  from "react-router-dom";

class FlowInspector extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    setDefaultTab = (tab) => {
        this.props.history.push(
            "/collected/" + this.props.flow.client_id + "/" +
                this.props.flow.session_id + "/" + tab);
    }

    render() {
        // Default tab comes from the router
        let default_tab = this.props.match && this.props.match.params &&
            this.props.match.params.tab;
        default_tab = default_tab || "overview";

        return (
            <div className="padded">
              <Tabs defaultActiveKey={default_tab} onSelect={this.setDefaultTab}>
                <Tab eventKey="overview" title="Artifact Collection">
                  <FlowOverview flow={this.props.flow}/>
                </Tab>
                <Tab eventKey="uploads" title="Uploaded Files">
                  <FlowUploads flow={this.props.flow}/>
                </Tab>
                <Tab eventKey="requests" title="Requests">
                  <FlowRequests flow={this.props.flow} />
                </Tab>
                <Tab eventKey="results" title="Results">
                  <FlowResults flow={this.props.flow} />
                </Tab>
                <Tab eventKey="logs" title="Log">
                </Tab>
              </Tabs>
              { JSON.stringify(this.props.flow)}
            </div>
        );
    }
};

export default withRouter(FlowInspector);
