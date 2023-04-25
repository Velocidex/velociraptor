import React from 'react';
import PropTypes from 'prop-types';
import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';
import T from '../i8n/i8n.jsx';
import FlowOverview from './flow-overview.jsx';
import FlowUploads from './flow-uploads.jsx';
import FlowRequests from './flow-requests.jsx';
import FlowResults from './flow-results.jsx';
import FlowLogs from './flow-logs.jsx';
import FlowNotebook from './flow-notebook.jsx';
import { withRouter }  from "react-router-dom";

// Typically subclassable by actual components.
import api from '../core/api-service.jsx';

import {CancelToken} from 'axios';

const POLL_TIME = 5000;

class FlowInspector extends React.Component {
    static propTypes = {
        flow: PropTypes.object,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        tab: "overview",
        detailed_flow: {context: {}},
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.fetchDetailedFlow, POLL_TIME);

        // Set the abbreviated flow in the meantime while we fetch the
        // full detailed to provide a smoother UX.
        this.setState({detailed_flow: {context: this.props.flow}});
        this.fetchDetailedFlow();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        // Update tab from router.
        let tab = this.props.match && this.props.match.params && this.props.match.params.tab;
        if (tab && tab !== this.state.tab) {
            this.setState({tab: tab});
            return true;
        }

        // Get the latest detailed flow if the flow id has changed.
        let prev_flow_id = prevProps.flow && prevProps.flow.session_id;
        if (this.props.flow.session_id !== prev_flow_id) {
            this.setState({detailed_flow: {context: this.props.flow}});
            this.fetchDetailedFlow();
            return true;
        }
        return false;
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    fetchDetailedFlow = () => {
        if (!this.props.flow || !this.props.flow.client_id) {
            return;
        };

        // Cancel any in flight requests.
        this.source.cancel();
        this.source = CancelToken.source();
        this.setState({loading: true});

        api.get("v1/GetFlowDetails", {
            flow_id: this.props.flow.session_id,
            client_id: this.props.flow.client_id,
        }, this.source.token).then((response) => {
            if (response.cancel) {
                return;
            };

            this.setState({detailed_flow: response.data, loading: false});
        });
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
            return <h5 className="no-content">
                     {T("Please click a collection in the above table")}
                   </h5>;
        }

        return (
            <div className="padded">
              <Tabs activeKey={this.state.tab}
                    onSelect={this.setDefaultTab}>
                <Tab eventKey="overview" title={T("Artifact Collection")}>
                  { this.state.tab === "overview" &&
                    <FlowOverview flow={this.state.detailed_flow.context}/>}
                </Tab>
                <Tab eventKey="uploads" title={T("Uploaded Files")}>
                  { this.state.tab === "uploads" &&
                    <FlowUploads flow={this.props.flow}/> }
                </Tab>
                <Tab eventKey="requests" title={T("Requests")}>
                  { this.state.tab === "requests" &&
                    <FlowRequests flow={this.props.flow} />}
                </Tab>
                <Tab eventKey="results" title={T("Results")}>
                  { this.state.tab === "results" &&
                    <FlowResults flow={this.state.detailed_flow.context} />}
                </Tab>
                <Tab eventKey="logs" title={T("Log")}>
                  { this.state.tab === "logs" &&
                    <FlowLogs flow={this.state.detailed_flow.context} />}
                </Tab>
                <Tab eventKey="notebook" title={T("Notebook")}>
                  { this.state.tab === "notebook" &&
                    <FlowNotebook flow={this.props.flow} />}
                </Tab>
              </Tabs>
            </div>
        );
    }
};

export default withRouter(FlowInspector);
