import React from 'react';
import PropTypes from 'prop-types';

import VeloAce from '../core/ace.js';

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

        if (flow_id != prev_flow_id) {
            this.fetchRequests();
        }
    }

    fetchRequests = () => {
        api.get("api/v1/GetFlowRequests", {
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
            <div className="card panel" >
              <h5 className="card-header">Request sent to client</h5>
              <div className="card-body">
                <VeloAce text={serialized}
                         options={options} />
              </div>
            </div>

        );
    }
};
