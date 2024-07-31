import React from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';
import VeloAce from '../core/ace.jsx';
import Row from 'react-bootstrap/Row';
import Card from 'react-bootstrap/Card';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';

export default class FlowRequests extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    state = {
        requests: [],
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchRequests();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let flow_id = this.props.flow && this.props.flow.session_id;
        let prev_flow_id = prevProps.flow && prevProps.flow.session_id;

        if (flow_id !== prev_flow_id) {
            this.fetchRequests();
        }
    }

    fetchRequests = () => {
        api.get("v1/GetFlowRequests", {
            flow_id: this.props.flow.session_id,
            client_id: this.props.flow.client_id,
        }, this.source.token).then((response) => {
            this.setState({requests: response.data.items});
        });
    }

    render() {
        let serialized = JSON.stringify(this.state.requests, null, 2);
        let options = {
            readOnly: true,
        };

        return (
            <Row>
              <Card>
                <Card.Header>{T("Request sent to client")}</Card.Header>
                <Card.Body>
                  <VeloAce text={serialized}
                           mode="json"
                           options={options} />
                </Card.Body>
              </Card>
            </Row>
        );
    }
};
