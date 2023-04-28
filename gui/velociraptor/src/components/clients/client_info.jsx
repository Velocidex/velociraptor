import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { withRouter }  from "react-router-dom";

import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';


// A component that syncs a client id to a client record.
class ClientSetterFromRoute extends Component {
    static propTypes = {
        client: PropTypes.object,
        setClient: PropTypes.func.isRequired,

        // React router props.
        match: PropTypes.object,
    }

    componentDidUpdate() {
        this.maybeUpdateClientInfo();
    }

    componentDidMount() {
        this.source = CancelToken.source();
        this.maybeUpdateClientInfo();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    maybeUpdateClientInfo() {
        let client_id = this.props.match && this.props.match.params &&
            this.props.match.params.client_id;

        if (!client_id) {
            return;
        };

        if (!this.props.client || client_id !== this.props.client.client_id) {
            if (client_id === "server") {
                window.globals.client = {client_id: "server"};
                this.props.setClient({client_id: "server"});
                return;
            }

            // We have no idea what this client is, just create an
            // empty record so the GUI can show something. This could
            // happen if the client is not in the index for some
            // reason so SearchClients can not find it, but it really
            // does exist.
            window.globals.client = {client_id: client_id};
            this.props.setClient({
                client_id: client_id,
            });

            api.get('v1/GetClient/' + client_id, {},
                    this.source.token).then(resp => {
                        if (resp.cancel) return;
                        if (resp.data && resp.data.client_id === client_id) {
                            window.globals.client = resp.data;
                            this.props.setClient(resp.data);
                        }
                    });
        }
    }

    render() {
        return <></>;
    }
}



export default withRouter(ClientSetterFromRoute);
