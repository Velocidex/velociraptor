import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';

import FlowOverview from './flow-overview.js';
import FlowUploads from './flow-uploads.js';
import FlowRequests from './flow-requests.js';
import FlowResults from './flow-results.js';
import FlowLogs from './flow-logs.js';
import FlowNotebook from './flow-notebook.js';
import { withRouter }  from "react-router-dom";

class FlowInspector extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    state = {
        tab: "overview",
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let tab = this.props.match && this.props.match.params && this.props.match.params.tab;
        if (tab && tab !== this.state.tab) {
            this.setState({tab: tab});
            return true;
        }
        return !_.isEqual(this.state, prevState) || !_.isEqual(this.props, prevProps);
    }

    setDefaultTab = (tab) => {
        this.setState({tab: tab});
        this.props.history.push(
            "/collected/" + this.props.flow.client_id + "/" +
                this.props.flow.session_id + "/" + tab);
    }

    render() {
        let flow = this.props.flow;
        let artifacts = flow && flow.request && flow.request.artifacts;

        if (!flow || !flow.session_id || !artifacts)  {
            return <h5 className="no-content">Please click a collection in the above table</h5>;
        }

        return (
            <div className="padded">
              <Tabs activeKey={this.state.tab}
                    onSelect={this.setDefaultTab}>
                <Tab eventKey="overview" title="Artifact Collection">
                  { this.state.tab === "overview" &&
                    <FlowOverview flow={this.props.flow}/>}
                </Tab>
                <Tab eventKey="uploads" title="Uploaded Files">
                  { this.state.tab === "uploads" &&
                    <FlowUploads flow={this.props.flow}/> }
                </Tab>
                <Tab eventKey="requests" title="Requests">
                  { this.state.tab === "requests" &&
                    <FlowRequests flow={this.props.flow} />}
                </Tab>
                <Tab eventKey="results" title="Results">
                  { this.state.tab === "results" &&
                    <FlowResults flow={this.props.flow} />}
                </Tab>
                <Tab eventKey="logs" title="Log">
                  { this.state.tab === "logs" &&
                    <FlowLogs flow={this.props.flow} />}
                </Tab>
                <Tab eventKey="notebook" title="Notebook">
                  { this.state.tab === "notebook" &&
                    <FlowNotebook flow={this.props.flow} />}
                </Tab>
              </Tabs>
            </div>
        );
    }
};

export default withRouter(FlowInspector);
