import React from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Spinner from '../utils/spinner.jsx';

import Modal from 'react-bootstrap/Modal';
import api from '../core/api-service.jsx';
import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';
import FlowOverview from './flow-overview.jsx';
import FlowUploads from './flow-uploads.jsx';
import FlowResults from './flow-results.jsx';
import FlowLogs from './flow-logs.jsx';
import {CancelToken} from 'axios';

class FlowPopupInspector extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    state = {
        // Show by default the logs tab.
        tab: "logs",
    }

    render() {
        let flow = this.props.flow;
        if (!flow || !flow.session_id)  {
            return <h5 className="no-content">
                     { T("Please click a collection in the above table") }
                   </h5>;
        }

        return (
            <div className="padded">
              <Tabs activeKey={this.state.tab}
                    onSelect={tab=>this.setState({tab: tab})}>
                <Tab eventKey="overview" title={T("Artifact Collection")}>
                  { this.state.tab === "overview" &&
                    <FlowOverview flow={this.props.flow}/>}
                </Tab>
                <Tab eventKey="uploads" title={T("Uploaded Files")}>
                  { this.state.tab === "uploads" &&
                    <FlowUploads flow={this.props.flow}/> }
                </Tab>
                <Tab eventKey="results" title={T("Results")}>
                  { this.state.tab === "results" &&
                    <FlowResults flow={this.props.flow} />}
                </Tab>
                <Tab eventKey="logs" title={T("Log")}>
                  { this.state.tab === "logs" &&
                    <FlowLogs flow={this.props.flow} />}
                </Tab>
              </Tabs>
            </div>
        );
    }
}


export default class FlowLink extends React.Component {
    static propTypes = {
        flow_id: PropTypes.string,
        client_id: PropTypes.string,
    };

    state = {
        showFlowInspector: false,
        flow: null,
        loading: false,
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    fetchFullFlow = () => {
        this.setState({showFlowInspector: true, loading: true});

        api.get("v1/GetFlowDetails", {
            flow_id: this.props.flow_id,
            client_id: this.props.client_id,
        }, this.source.token).then((response) => {
            this.setState({flow: response.data.context, loading: false});
        });
    }

    render() {
        return (
            <>
            { this.state.showFlowInspector &&
              <Modal show={true}
                     className="full-height"
                     dialogClassName="modal-90w"
                     enforceFocus={false}
                     scrollable={true}
                     onHide={() => this.setState({showFlowInspector: false})} >
                  <Modal.Header closeButton>
                    <Modal.Title>{T("Flow Details")}</Modal.Title>
                  </Modal.Header>
                  <Modal.Body>
                    <Spinner loading={this.state.loading}/>
                    <FlowPopupInspector flow={this.state.flow}/>
                  </Modal.Body>
                  <Modal.Footer>
                    <Button variant="secondary"
                            onClick={() => this.setState({showFlowInspector: false})}>
                      {T("Close")}
                    </Button>
                  </Modal.Footer>
                </Modal>
            }
              <Button size="sm" className="client-link"
                      onClick={this.fetchFullFlow}
                      variant="outline-info">
                <FontAwesomeIcon icon="external-link-alt"/>
                <span className="button-label">{ this.props.flow_id }</span>
              </Button>
            </>
        );
    }
};
