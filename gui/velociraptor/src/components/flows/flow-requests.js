import React from 'react';
import PropTypes from 'prop-types';

import VeloAce from '../core/ace.js';
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';

import api from '../core/api-service.js';

export default class FlowRequests extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    state = {
        requests: [],
    }

    componentDidMount = () => {
        this.fetchRequests();
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
        }).then((response) => {
            this.setState({requests: response.data.items});
        });
    }

    render() {
        let serialized = JSON.stringify(this.state.requests, null, 2);
        let options = {
            readOnly: true,
        };

        return (
            <CardDeck>
              <Card>
                <Card.Header>Request sent to client</Card.Header>
                <Card.Body>
                  <VeloAce text={serialized}
                           options={options} />
                </Card.Body>
              </Card>
            </CardDeck>
        );
    }
};
