import './snackbar.css';

import React from 'react';
import PropTypes from 'prop-types';

import { withSnackbar } from 'react-simple-snackbar';

import api from '../core/api-service.jsx';

class Snackbar extends React.Component {
    static propTypes = {
        openSnackbar: PropTypes.func,
    };

    componentDidMount = () => {
        api.hooks.push(this.warn);
    }

    warn = (message) => {
        this.props.openSnackbar(
            <div>{message}</div>, 5000);
    };

    render() {
        return (
            <>
            </>
        );
    }
};


export default withSnackbar(Snackbar, {
    position: 'bottom-right',
    style: {

    }});
