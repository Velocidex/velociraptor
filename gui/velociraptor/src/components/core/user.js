import React from 'react';
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
        api.get("v1/GetUserUITraits").then((response) => {
            let traits = response.data.interface_traits;
            traits.username = response.data.username;
            this.setState({traits: traits});

            document.body.classList.remove('dark-mode');
            document.body.classList.remove('light-mode');
            document.body.classList.add(traits.theme || "light-mode");
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

    getUserOptions = (traits) => {
        return JSON.parse(traits.ui_settings || "{}");
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
