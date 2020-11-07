import React from 'react';
import PropTypes from 'prop-types';

import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Spinner from '../utils/spinner.js';

import Modal from 'react-bootstrap/Modal';
import api from '../core/api-service.js';
import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';
import FlowOverview from './flow-overview.js';
import FlowUploads from './flow-uploads.js';
import FlowResults from './flow-results.js';
import FlowLogs from './flow-logs.js';


class FlowPopupInspector extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    state = {
        // Show by default the results tab.
        tab: "logs",
    }

    render() {
        let flow = this.props.flow;
        if (!flow || !flow.session_id)  {
            return <h5 className="no-content">Please click a collection in the above table</h5>;
        }

        return (
            <div className="padded">
              <Tabs activeKey={this.state.tab}
                    onSelect={tab=>this.setState({tab: tab})}>
                <Tab eventKey="overview" title="Artifact Collection">
                  { this.state.tab === "overview" &&
                    <FlowOverview flow={this.props.flow}/>}
                </Tab>
                <Tab eventKey="uploads" title="Uploaded Files">
                  { this.state.tab === "uploads" &&
                    <FlowUploads flow={this.props.flow}/> }
                </Tab>
                <Tab eventKey="results" title="Results">
                  { this.state.tab === "results" &&
                    <FlowResults flow={this.props.flow} />}
                </Tab>
                <Tab eventKey="logs" title="Log">
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

    fetchFullFlow = () => {
        this.setState({showFlowInspector: true, loading: true});

        api.get("v1/GetFlowDetails", {
            flow_id: this.props.flow_id,
            client_id: this.props.client_id,
        }).then((response) => {
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
                    <Modal.Title>Flow Details</Modal.Title>
                  </Modal.Header>
                  <Modal.Body>
                    <Spinner loading={this.state.loading}/>
                    <FlowPopupInspector flow={this.state.flow}/>
                  </Modal.Body>
                  <Modal.Footer>
                    <Button variant="secondary"
                            onClick={() => this.setState({showFlowInspector: false})}>
                      Close
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
