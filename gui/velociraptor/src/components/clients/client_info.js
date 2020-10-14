import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { withRouter }  from "react-router-dom";

import api from '../core/api-service.js';

// A component that syncs a client id to a client record.
class ClientSetterFromRoute extends Component {
    static propTypes = {
        client: PropTypes.object,
        setClient: PropTypes.func.isRequired,
    }

    componentDidUpdate() {
        this.maybeUpdateClientInfo();
    }

    componentDidMount() {
        this.maybeUpdateClientInfo();
    }

    maybeUpdateClientInfo() {
        let client_id = this.props.match && this.props.match.params &&
            this.props.match.params.client_id;

        if (!client_id) {
            return;
        };

        if (!this.props.client || client_id !== this.props.client.client_id) {
            if (client_id == "server") {
                this.props.setClient({client_id: "server"});
                return;
            }

            api.get('/api/v1/SearchClients', {
                query: client_id,
                count: 1,

            }).then(resp => {
                if (resp.data && resp.data.items) {
                    this.props.setClient(resp.data.items[0]);
                }
            });
        }
    }

    render() {
        return <></>;
    }
}



export default withRouter(ClientSetterFromRoute);
