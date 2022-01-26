import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { withRouter }  from "react-router-dom";

import api from '../core/api-service.js';
import axios from 'axios';


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
        this.source = axios.CancelToken.source();
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
                this.props.setClient({client_id: "server"});
                return;
            }

            api.get('/v1/SearchClients', {
                query: client_id,
            }, this.source.token).then(resp => {
                if (resp.data && resp.data.items) {
                    this.props.setClient(resp.data.items[0]);
                } else {
                    // We have no idea what this client is, just
                    // create an empty record so the GUI can show
                    // shomething. This could happen if the client is
                    // not in the index for some reason so
                    // SearchClients can not find it, but it really
                    // does exist.
                    this.props.setClient({
                        client_id: client_id,
                    });
                }
            });
        }
    }

    render() {
        return <></>;
    }
}



export default withRouter(ClientSetterFromRoute);
