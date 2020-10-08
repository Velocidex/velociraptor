import React from 'react';
import _ from 'lodash';

import api from '../core/api-service.js';
import axios from 'axios';

const UserConfig = React.createContext({
    traits: {},
    updateTraits: () => {},
});

const POLL_TIME = 5000;

// A component which maintains the user settings
export class UserSettings extends React.Component {
    updateTraits = () => {
        api.get("api/v1/GetUserUITraits").then((response) => {
            this.setState({traits: response.data.interface_traits});
        });
    }

    state = {
        traits: {},
        updateTraits: this.updateTraits,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.updateTraits, POLL_TIME);
        this.updateTraits();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    render() {
        return (
            <UserConfig.Provider value={this.state}>
              { this.props.children }
            </UserConfig.Provider>
        );
    }
};

export default UserConfig;
