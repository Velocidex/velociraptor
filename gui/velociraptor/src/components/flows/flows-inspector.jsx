import React from 'react';
import PropTypes from 'prop-types';
import Nav from 'react-bootstrap/Nav';
import T from '../i8n/i8n.jsx';
import FlowOverview from './flow-overview.jsx';
import FlowUploads from './flow-uploads.jsx';
import FlowRequests from './flow-requests.jsx';
import FlowResults from './flow-results.jsx';
import FlowLogs from './flow-logs.jsx';
import FlowNotebook from './flow-notebook.jsx';
import { withRouter }  from "react-router-dom";
import {getItem, setItem, schema} from '../core/storage.jsx';

// Typically subclassable by actual components.
import api from '../core/api-service.jsx';

import {CancelToken} from 'axios';

const POLL_TIME = 5000;

class FlowInspector extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
        refreshFlows: PropTypes.func,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {
        tab: "overview",
    }

    componentDidMount = () => {
        this.source = CancelToken.source();

        // Update tab from router.
        let tab = this.props.match &&
            this.props.match.params && this.props.match.params.tab;
        if (tab && tab !== this.state.tab) {
            this.setDefaultTab(tab);
        }
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    XXXcomponentDidUpdate(prevProps, prevState, snapshot) {

    }

    setDefaultTab = (tab) => {
        let flow = this.props.flow;
        let client_id = flow.client_id;
        let flow_id = flow.session_id;
        if(!client_id || !flow_id) {
            return;
        }

        setItem(schema.ClientFlowsTabKey, tab);
        this.setState({tab: tab});
        this.props.history.push(
            "/collected/" + client_id + "/" + flow_id + "/" + tab);
    }

    render() {
        let flow = this.props.flow;
        let artifacts = flow && flow.request && flow.request.artifacts;
        let tab = getItem(schema.ClientFlowsTabKey) || this.state.tab;

        if (!flow || !flow.session_id || !artifacts)  {
            return <h5 className="no-content">
                     {T("Please click a collection in the above table")}
                   </h5>;
        }

        return (
            <div className="padded">
              <Nav variant="tab" activeKey={tab}
                   onSelect={this.setDefaultTab}>
                <Nav.Item>
                  <Nav.Link as="button" className="btn btn-default"
                            eventKey="overview">{T("Artifact Collection")}</Nav.Link>
                </Nav.Item>
                <Nav.Item>
                  <Nav.Link as="button" className="btn btn-default"
                            eventKey="uploads">{T("Uploaded Files")}</Nav.Link>
                </Nav.Item>
                <Nav.Item>
                  <Nav.Link as="button" className="btn btn-default"
                            eventKey="requests">{T("Requests")}</Nav.Link>
                </Nav.Item>
                <Nav.Item>
                  <Nav.Link as="button" className="btn btn-default"
                            eventKey="results">{T("Results")}</Nav.Link>
                </Nav.Item>
                <Nav.Item>
                  <Nav.Link as="button" className="btn btn-default"
                            eventKey="logs">{T("Log")}</Nav.Link>
                </Nav.Item>
                <Nav.Item>
                  <Nav.Link as="button" className="btn btn-default"
                            eventKey="notebook">{T("Notebook")}</Nav.Link>
                </Nav.Item>
              </Nav>
              <div className="card-deck">
                { tab === "overview" &&
                  <FlowOverview
                    refreshFlows={this.props.refreshFlows}
                    flow={flow}/>}
                { tab === "uploads" &&
                  <FlowUploads flow={flow}/> }
                { tab === "requests" &&
                  <FlowRequests flow={flow} />}
                { tab === "results" &&
                  <FlowResults flow={flow} />}
                { tab === "logs" &&
                  <FlowLogs flow={flow} />}
                { tab === "notebook" &&
                  <FlowNotebook flow={flow} />}
              </div>
            </div>
        );
    }
};

export default withRouter(FlowInspector);
